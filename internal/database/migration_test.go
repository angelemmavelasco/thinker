package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestInitSchema_And_BackfillIdeaGroups(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(ctx, db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Insert ideas requiring backfill: one with nil UUID, one with empty text
	userID := uuid.New().String()
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role, auth_provider, email_verified, token_version)
		VALUES ($1, 'test@test.local', 'hash', 'user', 'local', 1, 1);
	`, userID)
	if err != nil {
		t.Fatalf("failed inserting test user: %v", err)
	}

	idea1ID := uuid.New().String()
	idea2ID := uuid.New().String()
	_, err = db.ExecContext(ctx, `
		INSERT INTO ideas (id, user_id, group_id, chunk_index, chunk_total, title, content, status)
		VALUES 
			($1, $2, '00000000-0000-0000-0000-000000000000', 0, 0, 'Document A (1/2)', 'Content 1', 'pendiente'),
			($3, $2, '', 0, 0, 'Document A (2/2)', 'Content 2', 'pendiente');
	`, idea1ID, userID, idea2ID)
	if err != nil {
		t.Fatalf("failed inserting test ideas: %v", err)
	}

	// Run backfillIdeaGroups
	if err := backfillIdeaGroups(ctx, db); err != nil {
		t.Fatalf("backfillIdeaGroups failed: %v", err)
	}

	// Verify both chunks got assigned the same new group_id
	var group1, group2 string
	var idx1, idx2, tot1, tot2 int
	err = db.QueryRowContext(ctx, "SELECT group_id, chunk_index, chunk_total FROM ideas WHERE id = $1", idea1ID).Scan(&group1, &idx1, &tot1)
	if err != nil {
		t.Fatalf("failed querying idea1: %v", err)
	}
	err = db.QueryRowContext(ctx, "SELECT group_id, chunk_index, chunk_total FROM ideas WHERE id = $1", idea2ID).Scan(&group2, &idx2, &tot2)
	if err != nil {
		t.Fatalf("failed querying idea2: %v", err)
	}

	if group1 == "00000000-0000-0000-0000-000000000000" || group1 == "" {
		t.Errorf("idea1 group_id was not backfilled: %s", group1)
	}
	if group1 != group2 {
		t.Errorf("expected both chunks of Document A to share group_id, got %s and %s", group1, group2)
	}
	if idx1 != 1 || tot1 != 2 || idx2 != 2 || tot2 != 2 {
		t.Errorf("chunk index/total mismatch: (%d/%d), (%d/%d)", idx1, tot1, idx2, tot2)
	}
}

func TestInitSchema_HierarchicalMOCs(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:moc_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(ctx, db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	queries := New(db)

	// Verify default settings seeded
	setting, err := queries.GetKnowledgeSetting(ctx, "allow_auto_moc_creation")
	if err != nil {
		t.Fatalf("expected allow_auto_moc_creation to be seeded, got: %v", err)
	}
	if setting.Value != "true" {
		t.Errorf("expected value 'true', got %s", setting.Value)
	}

	// Create MOC
	mocID := uuid.New()
	moc, err := queries.CreateKnowledgeMOC(ctx, CreateKnowledgeMOCParams{
		ID:           mocID,
		Code:         "MOC_CANALES",
		Title:        "Canales y Restricciones",
		Description:  "Requisitos sanitarios para venta a estéticas y clínicas",
		Embedding:    "[0.1, 0.2]",
		DisplayOrder: 1,
		IsLocked:     0,
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeMOC failed: %v", err)
	}
	if moc.Code != "MOC_CANALES" {
		t.Errorf("unexpected code: %s", moc.Code)
	}

	// Create Hub
	hubID := uuid.New()
	hub, err := queries.CreateKnowledgeHub(ctx, CreateKnowledgeHubParams{
		ID:            hubID,
		MocID:         mocID,
		Title:         "Restricciones por Giro Comercial",
		Summary:       "Estéticas no pueden adquirir dietas patológicas",
		SearchAliases: `["peluqueria", "grooming"]`,
		Embedding:     "[0.3, 0.4]",
		DisplayOrder:  1,
		IsLocked:      0,
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeHub failed: %v", err)
	}
	if hub.Title != "Restricciones por Giro Comercial" {
		t.Errorf("unexpected title: %s", hub.Title)
	}

	// Create User & Idea to link
	userID := uuid.New()
	_, err = queries.CreateUser(ctx, CreateUserParams{
		ID:            userID,
		Email:         "moc_tester@test.local",
		PasswordHash:  "hash",
		Role:          "member",
		AuthProvider:  "local",
		EmailVerified: 1,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	ideaID := uuid.New()
	_, err = queries.CreateIdea(ctx, CreateIdeaParams{
		ID:         ideaID,
		UserID:     userID,
		GroupID:    ideaID,
		ChunkIndex: 1,
		ChunkTotal: 1,
		Title:      "Cláusula 24: Venta a Estéticas",
		Content:    "Queda prohibida la venta de dietas de prescripción médica a estéticas caninas.",
		Status:     "procesada",
	})
	if err != nil {
		t.Fatalf("CreateIdea failed: %v", err)
	}

	// Link Hub to Idea
	edge, err := queries.CreateHubIdeaEdge(ctx, CreateHubIdeaEdgeParams{
		HubID:            hubID,
		IdeaID:           ideaID,
		RelationshipType: "GOVERNS",
		Weight:           1.0,
		IsManualOverride: 0,
	})
	if err != nil {
		t.Fatalf("CreateHubIdeaEdge failed: %v", err)
	}
	if edge.RelationshipType != "GOVERNS" {
		t.Errorf("unexpected relation: %s", edge.RelationshipType)
	}

	// Verify listing ideas by Hub
	ideas, err := queries.ListIdeasByHubID(ctx, hubID)
	if err != nil {
		t.Fatalf("ListIdeasByHubID failed: %v", err)
	}
	if len(ideas) != 1 || ideas[0].ID != ideaID {
		t.Fatalf("expected 1 idea with id %v, got %d", ideaID, len(ideas))
	}

	// Verify cascade deletion
	err = queries.DeleteKnowledgeMOC(ctx, mocID)
	if err != nil {
		t.Fatalf("DeleteKnowledgeMOC failed: %v", err)
	}

	hubs, err := queries.ListKnowledgeHubsByMOCID(ctx, mocID)
	if err != nil {
		t.Fatalf("ListKnowledgeHubsByMOCID failed: %v", err)
	}
	if len(hubs) != 0 {
		t.Errorf("expected 0 hubs after MOC deletion due to CASCADE, got %d", len(hubs))
	}
}
