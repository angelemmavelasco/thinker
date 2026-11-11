package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"thinker/internal/database"
)

func setupTestHierarchicalService(t *testing.T) (*IdeaService, *database.Queries, uuid.UUID, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:hierarchical_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
		Email:         fmt.Sprintf("hier_user_%d@test.com", time.Now().UnixNano()),
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

	ideaSvc := NewIdeaService(queries, nil, nil)

	cleanup := func() {
		_ = conn.Close()
	}

	return ideaSvc, queries, testUserID, cleanup
}

func TestHierarchicalQuery_PetShopDietResolution_EndToEnd(t *testing.T) {
	svc, q, userID, cleanup := setupTestHierarchicalService(t)
	defer cleanup()

	ctx := context.Background()

	// Mock LLM: handles Triage, ExpandQuery, and AskKnowledgeGraph
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			var req chatCompletionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			var content string
			if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "clasificador de índices") {
				// MOC Triage response
				content = `{"target_mocs": ["MOC_CANALES"]}`
			} else if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "Grounding estricto") {
				// AskKnowledgeGraph response
				content = "Las estéticas y peluquerías caninas tienen prohibida la adquisición de dietas patológicas conforme a la Cláusula 24."
			} else {
				content = "estética canina dietas prescripcion"
			}

			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: content,
						},
					},
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

	llm := NewLLMService("http://mock-llm.local", "key")
	llm.SetHTTPClient(mockClient)
	svc.llm = llm
	svc.topicService.llm = llm

	// Mock embeddings
	vecCanales, _ := SerializeVector([]float64{0.95, 0.15, 0.0})
	mockEmbClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{
					{Embedding: []float64{0.95, 0.15, 0.0}},
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
	embSvc := NewEmbeddingService("http://mock-embedding.local", "key")
	embSvc.SetHTTPClient(mockEmbClient)
	svc.embedding = embSvc
	svc.topicService.embedding = embSvc

	// Setup MOC and Hub
	moc, err := svc.topicService.CreateMOC(ctx, "MOC_CANALES", "Canales y Restricciones Sanitarias", "Giro de clientes y permisos", false)
	if err != nil {
		t.Fatalf("CreateMOC failed: %v", err)
	}

	hub, err := svc.topicService.CreateHub(ctx, moc.ID, "Restricciones por Giro Comercial", "Reglas para estéticas y clínicas", []string{"estetica", "peluqueria", "grooming"}, false)
	if err != nil {
		t.Fatalf("CreateHub failed: %v", err)
	}

	// Create Clause 24 and link to Hub
	clause24 := createTestIdea(ctx, t, q, userID, "Cláusula 24: Venta a Estéticas", "Queda estrictamente prohibida la venta de dietas de prescripción a estéticas o peluquerías caninas.")
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  clause24.ID,
		RestructuredContent: clause24.Content,
		Embedding:           vecCanales,
		Status:              "procesada",
	})
	_ = svc.topicService.LinkIdeaToHub(ctx, hub.ID, clause24.ID, "GOVERNS", false)

	// User query
	userQuery := "¿un negocio de estética que pide dietas patológicas puede comprarlas?"
	res, err := svc.QueryGraph(ctx, userQuery)
	if err != nil {
		t.Fatalf("QueryGraph failed: %v", err)
	}

	if len(res.Seeds) == 0 {
		t.Fatalf("expected at least 1 seed, got 0")
	}
	if res.Seeds[0].IdeaID != clause24.ID {
		t.Errorf("expected seed to be Clause 24, got %v (%s)", res.Seeds[0].IdeaID, res.Seeds[0].Title)
	}
	if !strings.Contains(res.Answer, "Cláusula 24") {
		t.Errorf("expected answer to reference Clause 24, got: %s", res.Answer)
	}
}

func TestHierarchicalQuery_FallbackWhenNoMOCs(t *testing.T) {
	svc, q, userID, cleanup := setupTestHierarchicalService(t)
	defer cleanup()

	ctx := context.Background()

	// No MOCs created in DB!
	vecSeed, _ := SerializeVector([]float64{0.95, 0.15, 0.0})
	idea := createTestIdea(ctx, t, q, userID, "Nota de Arquitectura", "Diseño de sistemas distribuidos y microservicios")
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  idea.ID,
		RestructuredContent: idea.Content,
		Embedding:           vecSeed,
		Status:              "procesada",
	})

	mockEmbClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{
					{Embedding: []float64{0.95, 0.15, 0.0}},
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
	embSvc := NewEmbeddingService("http://mock-embedding.local", "key")
	embSvc.SetHTTPClient(mockEmbClient)
	svc.embedding = embSvc

	// Call SearchAndExpand: should fall back smoothly to FindSeedIdeas
	opts := DefaultTraversalOptions()
	subGraph, seeds, err := svc.SearchAndExpand(ctx, "microservicios distribuidos", 3, opts)
	if err != nil {
		t.Fatalf("SearchAndExpand failed: %v", err)
	}

	if len(seeds) == 0 {
		t.Fatalf("expected fallback seeds to be found, got 0")
	}
	if seeds[0].IdeaID != idea.ID {
		t.Errorf("expected fallback idea to match, got %v", seeds[0].IdeaID)
	}
	if len(subGraph.Nodes) == 0 {
		t.Errorf("expected subgraph nodes to be populated from fallback")
	}
}
