package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"thinker/internal/database"
)

type mockRoundTripper func(*http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return m(r)
}

func setupTestSeedService(t *testing.T) (*IdeaService, *database.Queries, uuid.UUID, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:seed_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
		Email:         fmt.Sprintf("seed_user_%d@test.com", time.Now().UnixNano()),
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

	kwSvc := NewKeywordService(queries, nil)
	ideaSvc := &IdeaService{
		queries:        queries,
		keywordService: kwSvc,
		jobQueue:       make(chan ideaJob, 10),
	}

	cleanup := func() {
		_ = conn.Close()
	}

	return ideaSvc, queries, testUserID, cleanup
}

func TestFindSeedIdeas_EmptyOrBlank(t *testing.T) {
	svc, _, _, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	res, err := svc.FindSeedIdeas(ctx, "", 3)
	if err != nil {
		t.Fatalf("unexpected error for empty query: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results for empty query, got %d", len(res))
	}

	res, err = svc.FindSeedIdeas(ctx, "     ", 3)
	if err != nil {
		t.Fatalf("unexpected error for blank query: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results for blank query, got %d", len(res))
	}
}

func TestFindSeedIdeas_KeywordMatchingOnly(t *testing.T) {
	svc, q, userID, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	idea1 := createTestIdea(ctx, t, q, userID, "Arquitectura de Software Modular", "Notas sobre microservicios y acoplamiento")
	idea2 := createTestIdea(ctx, t, q, userID, "Recetas de Cocina Tradicional", "Pastas caseras y salsas italianas")
	idea3 := createTestIdea(ctx, t, q, userID, "Criptografia y Seguridad Web", "Tokens JWT y almacenamiento seguro de hashes")

	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, idea1.ID, []string{"modularidad", "arquitectura"})
	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, idea2.ID, []string{"cocina", "pastas"})
	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, idea3.ID, []string{"criptografia", "seguridad", "jwt"})

	results, err := svc.FindSeedIdeas(ctx, "seguridad en criptografia con tokens jwt", 3)
	if err != nil {
		t.Fatalf("FindSeedIdeas failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least 1 seed candidate, got 0")
	}

	if results[0].IdeaID != idea3.ID {
		t.Fatalf("expected idea3 to be top seed, got %s (%s)", results[0].IdeaID, results[0].Title)
	}

	if results[0].KeywordScore <= 0 {
		t.Errorf("expected positive keyword score, got %f", results[0].KeywordScore)
	}

	if len(results[0].MatchedTerms) == 0 {
		t.Errorf("expected matched terms for top candidate")
	}
}

func TestFindSeedIdeas_VectorSimilarity(t *testing.T) {
	svc, q, userID, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{
					{Embedding: []float64{1.0, 0.0, 0.0}},
				},
			}
			raw, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(raw)),
			}, nil
		}),
	}

	embSvc := NewEmbeddingService("http://mock-embedding.local", "test-key")
	embSvc.SetHTTPClient(mockClient)
	svc.embedding = embSvc

	vecClose, _ := SerializeVector([]float64{0.99, 0.01, 0.0})
	vecDistant, _ := SerializeVector([]float64{0.0, 1.0, 0.0})

	ideaClose := createTestIdea(ctx, t, q, userID, "Tema de Matematica", "Calculo vectorial avanzado")
	ideaDistant := createTestIdea(ctx, t, q, userID, "Tema de Biologia", "Genetica molecular")

	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaClose.ID,
		RestructuredContent: ideaClose.Content,
		Embedding:           vecClose,
		Status:              "procesada",
	})

	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaDistant.ID,
		RestructuredContent: ideaDistant.Content,
		Embedding:           vecDistant,
		Status:              "procesada",
	})

	results, err := svc.FindSeedIdeas(ctx, "matematica y calculo", 2)
	if err != nil {
		t.Fatalf("FindSeedIdeas failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected seed results, got 0")
	}

	if results[0].IdeaID != ideaClose.ID {
		t.Fatalf("expected ideaClose to be ranked first, got %s", results[0].Title)
	}

	if results[0].VectorScore < 0.90 {
		t.Errorf("expected vector score > 0.90, got %f", results[0].VectorScore)
	}
}

func TestSearchAndExpand_EndToEnd(t *testing.T) {
	svc, q, userID, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	seedIdea := createTestIdea(ctx, t, q, userID, "Redes Neuronales y Grafos", "Estructuras asociativas")
	hop1Idea := createTestIdea(ctx, t, q, userID, "Algoritmos de PageRank", "Calculo de importancia")
	hop2Idea := createTestIdea(ctx, t, q, userID, "Buscadores Web", "Indexacion y ranking")

	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, seedIdea.ID, []string{"neuronales", "grafos"})
	linkTestIdeas(ctx, t, q, seedIdea.ID, hop1Idea.ID, "semantic", 0.90)
	linkTestIdeas(ctx, t, q, hop1Idea.ID, hop2Idea.ID, "semantic", 0.85)

	opts := DefaultTraversalOptions()
	subGraph, seeds, err := svc.SearchAndExpand(ctx, "grafos y redes asociativas", 1, opts)
	if err != nil {
		t.Fatalf("SearchAndExpand failed: %v", err)
	}

	if len(seeds) != 1 || seeds[0].IdeaID != seedIdea.ID {
		t.Fatalf("expected seedIdea as primary seed, got %+v", seeds)
	}

	if len(subGraph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes in 2-hop traversal, got %d", len(subGraph.Nodes))
	}

	distMap := make(map[uuid.UUID]int)
	for _, n := range subGraph.Nodes {
		distMap[n.ID] = n.HopDistance
	}

	if distMap[seedIdea.ID] != 0 {
		t.Errorf("expected seed distance 0, got %d", distMap[seedIdea.ID])
	}
	if distMap[hop1Idea.ID] != 1 {
		t.Errorf("expected hop1 distance 1, got %d", distMap[hop1Idea.ID])
	}
	if distMap[hop2Idea.ID] != 2 {
		t.Errorf("expected hop2 distance 2, got %d", distMap[hop2Idea.ID])
	}
}
