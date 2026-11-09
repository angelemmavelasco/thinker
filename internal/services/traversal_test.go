package services

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"thinker/internal/database"
)

func setupTestIdeaService(t *testing.T) (*IdeaService, *database.Queries, uuid.UUID, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:traversal_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed opening sqlite: %v", err)
	}

	ctx := context.Background()
	if err := database.InitSchema(ctx, conn); err != nil {
		_ = conn.Close()
		t.Fatalf("InitSchema failed: %v", err)
	}

	queries := database.New(conn)
	testUserID := uuid.New()
	_, err = queries.CreateUser(ctx, database.CreateUserParams{
		ID:            testUserID,
		Email:         fmt.Sprintf("user_%d@test.com", time.Now().UnixNano()),
		PasswordHash:  "hash",
		Role:          "member",
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 1,
	})
	if err != nil {
		_ = conn.Close()
		t.Fatalf("CreateUser failed: %v", err)
	}

	ideaSvc := &IdeaService{
		queries:  queries,
		jobQueue: make(chan ideaJob, 10),
	}

	cleanup := func() {
		_ = conn.Close()
	}

	return ideaSvc, queries, testUserID, cleanup
}

func createTestIdea(ctx context.Context, t *testing.T, q *database.Queries, userID uuid.UUID, title, content string) database.Idea {
	t.Helper()
	id := uuid.New()
	idea, err := q.CreateIdea(ctx, database.CreateIdeaParams{
		ID:         id,
		UserID:     userID,
		GroupID:    id,
		ChunkIndex: 1,
		ChunkTotal: 1,
		Title:      title,
		Content:    content,
		Status:     "procesada",
	})
	if err != nil {
		t.Fatalf("CreateIdea failed: %v", err)
	}
	return idea
}

func linkTestIdeas(ctx context.Context, t *testing.T, q *database.Queries, sourceID, targetID uuid.UUID, relType string, weight float64) {
	t.Helper()
	_, err := q.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
		ID:           uuid.New(),
		SourceIdeaID: sourceID,
		TargetIdeaID: targetID,
		RelationType: relType,
		Weight:       weight,
	})
	if err != nil {
		t.Fatalf("CreateIdeaLink forward failed: %v", err)
	}

	_, err = q.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
		ID:           uuid.New(),
		SourceIdeaID: targetID,
		TargetIdeaID: sourceID,
		RelationType: relType,
		Weight:       weight,
	})
	if err != nil {
		t.Fatalf("CreateIdeaLink backward failed: %v", err)
	}
}

func TestTraverseSubGraph_LinearChain(t *testing.T) {
	svc, q, userID, cleanup := setupTestIdeaService(t)
	defer cleanup()

	ctx := context.Background()

	ideaA := createTestIdea(ctx, t, q, userID, "Idea A", "Contenido A")
	ideaB := createTestIdea(ctx, t, q, userID, "Idea B", "Contenido B")
	ideaC := createTestIdea(ctx, t, q, userID, "Idea C", "Contenido C")
	ideaD := createTestIdea(ctx, t, q, userID, "Idea D", "Contenido D")

	linkTestIdeas(ctx, t, q, ideaA.ID, ideaB.ID, "semantic", 0.90)
	linkTestIdeas(ctx, t, q, ideaB.ID, ideaC.ID, "semantic", 0.80)
	linkTestIdeas(ctx, t, q, ideaC.ID, ideaD.ID, "semantic", 0.70)

	opts := DefaultTraversalOptions()
	subGraph, err := svc.TraverseSubGraph(ctx, []uuid.UUID{ideaA.ID}, opts)
	if err != nil {
		t.Fatalf("TraverseSubGraph failed: %v", err)
	}

	if len(subGraph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (A, B, C), got %d", len(subGraph.Nodes))
	}

	nodeMap := make(map[uuid.UUID]TraversalNode)
	for _, n := range subGraph.Nodes {
		nodeMap[n.ID] = n
	}

	nodeA, okA := nodeMap[ideaA.ID]
	if !okA || nodeA.HopDistance != 0 || !nodeA.IsSeed() {
		t.Errorf("node A invalid: %+v", nodeA)
	}

	nodeB, okB := nodeMap[ideaB.ID]
	if !okB || nodeB.HopDistance != 1 || nodeB.LinkedFromID != ideaA.ID {
		t.Errorf("node B invalid: %+v", nodeB)
	}

	nodeC, okC := nodeMap[ideaC.ID]
	if !okC || nodeC.HopDistance != 2 || nodeC.LinkedFromID != ideaB.ID {
		t.Errorf("node C invalid: %+v", nodeC)
	}

	if _, okD := nodeMap[ideaD.ID]; okD {
		t.Errorf("node D should not be present in 2-hop traversal")
	}

	if len(subGraph.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(subGraph.Edges))
	}
}

func TestTraverseSubGraph_CycleAndDuplicatePrevention(t *testing.T) {
	svc, q, userID, cleanup := setupTestIdeaService(t)
	defer cleanup()

	ctx := context.Background()

	ideaA := createTestIdea(ctx, t, q, userID, "Idea A", "Contenido A")
	ideaB := createTestIdea(ctx, t, q, userID, "Idea B", "Contenido B")
	ideaC := createTestIdea(ctx, t, q, userID, "Idea C", "Contenido C")

	linkTestIdeas(ctx, t, q, ideaA.ID, ideaB.ID, "semantic", 0.90)
	linkTestIdeas(ctx, t, q, ideaB.ID, ideaC.ID, "semantic", 0.85)
	linkTestIdeas(ctx, t, q, ideaC.ID, ideaA.ID, "concept_overlap", 0.80)

	opts := DefaultTraversalOptions()
	subGraph, err := svc.TraverseSubGraph(ctx, []uuid.UUID{ideaA.ID}, opts)
	if err != nil {
		t.Fatalf("TraverseSubGraph failed: %v", err)
	}

	if len(subGraph.Nodes) != 3 {
		t.Fatalf("expected exactly 3 nodes without duplicates, got %d", len(subGraph.Nodes))
	}

	seen := make(map[uuid.UUID]int)
	for _, n := range subGraph.Nodes {
		seen[n.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("node %s appeared %d times", id, count)
		}
	}
}

func TestTraverseSubGraph_WeightPruningAndNodeLimits(t *testing.T) {
	svc, q, userID, cleanup := setupTestIdeaService(t)
	defer cleanup()

	ctx := context.Background()

	ideaA := createTestIdea(ctx, t, q, userID, "Seed A", "Contenido A")
	n1 := createTestIdea(ctx, t, q, userID, "N1", "Contenido N1")
	n2 := createTestIdea(ctx, t, q, userID, "N2", "Contenido N2")
	n3 := createTestIdea(ctx, t, q, userID, "N3", "Contenido N3")
	n4 := createTestIdea(ctx, t, q, userID, "N4", "Contenido N4")

	linkTestIdeas(ctx, t, q, ideaA.ID, n1.ID, "semantic", 0.95)
	linkTestIdeas(ctx, t, q, ideaA.ID, n2.ID, "semantic", 0.85)
	linkTestIdeas(ctx, t, q, ideaA.ID, n3.ID, "semantic", 0.50)
	linkTestIdeas(ctx, t, q, ideaA.ID, n4.ID, "semantic", 0.05)

	opts := TraversalOptions{
		Max1HopPerNode: 2,
		Max2HopPerNode: 2,
		MaxTotalNodes:  5,
		MinWeight:      0.10,
	}

	subGraph, err := svc.TraverseSubGraph(ctx, []uuid.UUID{ideaA.ID}, opts)
	if err != nil {
		t.Fatalf("TraverseSubGraph failed: %v", err)
	}

	if len(subGraph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (Seed, N1, N2), got %d", len(subGraph.Nodes))
	}

	nodeMap := make(map[uuid.UUID]bool)
	for _, n := range subGraph.Nodes {
		nodeMap[n.ID] = true
	}

	if !nodeMap[ideaA.ID] || !nodeMap[n1.ID] || !nodeMap[n2.ID] {
		t.Errorf("expected A, N1 and N2 in subGraph, got nodes: %+v", subGraph.Nodes)
	}
	if nodeMap[n3.ID] {
		t.Errorf("N3 should have been pruned by Max1HopPerNode limit")
	}
	if nodeMap[n4.ID] {
		t.Errorf("N4 should have been pruned by MinWeight threshold")
	}
}

func TestTraverseSubGraph_MultipleSeeds(t *testing.T) {
	svc, q, userID, cleanup := setupTestIdeaService(t)
	defer cleanup()

	ctx := context.Background()

	seedA := createTestIdea(ctx, t, q, userID, "Seed A", "Contenido A")
	seedB := createTestIdea(ctx, t, q, userID, "Seed B", "Contenido B")
	sharedX := createTestIdea(ctx, t, q, userID, "Shared X", "Contenido X")
	uniqueY := createTestIdea(ctx, t, q, userID, "Unique Y", "Contenido Y")

	linkTestIdeas(ctx, t, q, seedA.ID, sharedX.ID, "semantic", 0.90)
	linkTestIdeas(ctx, t, q, seedB.ID, sharedX.ID, "semantic", 0.85)
	linkTestIdeas(ctx, t, q, seedB.ID, uniqueY.ID, "concept_overlap", 0.75)

	opts := DefaultTraversalOptions()
	subGraph, err := svc.TraverseSubGraph(ctx, []uuid.UUID{seedA.ID, seedB.ID}, opts)
	if err != nil {
		t.Fatalf("TraverseSubGraph failed: %v", err)
	}

	if len(subGraph.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(subGraph.Nodes))
	}

	if len(subGraph.SeedIDs) != 2 {
		t.Fatalf("expected 2 seed IDs, got %d", len(subGraph.SeedIDs))
	}

	xCount := 0
	for _, n := range subGraph.Nodes {
		if n.ID == sharedX.ID {
			xCount++
		}
	}
	if xCount != 1 {
		t.Errorf("shared node X should appear exactly once, got %d", xCount)
	}
}

func TestTraverseSubGraph_EmptyOrMissing(t *testing.T) {
	svc, _, _, cleanup := setupTestIdeaService(t)
	defer cleanup()

	ctx := context.Background()

	subGraph, err := svc.TraverseSubGraph(ctx, nil, DefaultTraversalOptions())
	if err != nil {
		t.Fatalf("unexpected error for nil seeds: %v", err)
	}
	if len(subGraph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(subGraph.Nodes))
	}

	missingID := uuid.New()
	subGraph, err = svc.TraverseSubGraph(ctx, []uuid.UUID{missingID}, DefaultTraversalOptions())
	if err != nil {
		t.Fatalf("unexpected error for missing seed: %v", err)
	}
	if len(subGraph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes for non-existent seed, got %d", len(subGraph.Nodes))
	}
}
