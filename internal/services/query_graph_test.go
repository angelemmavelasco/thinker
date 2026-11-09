package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"thinker/internal/database"
)

func TestBuildSubGraphPrompt(t *testing.T) {
	idA := uuid.New()
	idB := uuid.New()
	idC := uuid.New()

	subGraph := &TraversalSubGraph{
		SeedIDs: []uuid.UUID{idA},
		Nodes: []TraversalNode{
			{
				ID:                  idA,
				Title:               "Arquitectura Modular",
				Content:             "Contenido crudo A",
				RestructuredContent: "Notas sobre desacoplamiento e interfaces.",
				HopDistance:         0,
				LinkedFromID:        uuid.Nil,
				RelationType:        "seed",
				Weight:              1.0,
			},
			{
				ID:                  idB,
				Title:               "Seguridad de Tokens",
				Content:             "Contenido B",
				RestructuredContent: "Gestion de claves asimetricas y JWT.",
				HopDistance:         1,
				LinkedFromID:        idA,
				RelationType:        "semantic",
				Weight:              0.88,
			},
			{
				ID:                  idC,
				Title:               "Rotacion de Refresh Tokens",
				Content:             "Contenido C",
				RestructuredContent: "Deteccion de robo de tokens.",
				HopDistance:         2,
				LinkedFromID:        idB,
				RelationType:        "concept_overlap",
				Weight:              0.82,
			},
		},
		Edges: []TraversalEdge{
			{
				SourceID:     idA,
				TargetID:     idB,
				RelationType: "semantic",
				Weight:       0.88,
				HopLevel:     1,
			},
			{
				SourceID:     idB,
				TargetID:     idC,
				RelationType: "concept_overlap",
				Weight:       0.82,
				HopLevel:     2,
			},
		},
	}

	prompt := BuildSubGraphPrompt(subGraph)

	if !strings.Contains(prompt, "Contexto primario (Nodos Semilla):") {
		t.Fatalf("expected header for primary context in prompt")
	}
	if !strings.Contains(prompt, "- [Arquitectura Modular]: Notas sobre desacoplamiento e interfaces.") {
		t.Errorf("expected seed node in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Contexto asociativo conectado (Vecinos directos y secundarios):") {
		t.Fatalf("expected header for associative context in prompt")
	}
	if !strings.Contains(prompt, "- [Seguridad de Tokens] (conectada con Arquitectura Modular por 'semantic'): Gestion de claves asimetricas y JWT.") {
		t.Errorf("expected 1-hop connected node in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "- [Rotacion de Refresh Tokens] (conectada con Seguridad de Tokens por 'concept_overlap'): Deteccion de robo de tokens.") {
		t.Errorf("expected 2-hop connected node in prompt, got:\n%s", prompt)
	}
}

func TestAskKnowledgeGraph_MockLLM(t *testing.T) {
	ctx := context.Background()

	expectedAnswer := "La arquitectura modular se conecta con la seguridad a traves del manejo de tokens asimetricos."

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: expectedAnswer,
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

	llm := NewLLMService("http://mock-llm.local", "test-key")
	llm.SetHTTPClient(mockClient)

	subGraph := &TraversalSubGraph{
		Nodes: []TraversalNode{
			{
				ID:          uuid.New(),
				Title:       "Idea A",
				Content:     "Texto de Idea A",
				HopDistance: 0,
			},
		},
	}

	ans, err := llm.AskKnowledgeGraph(ctx, "¿Como se relacionan los modulos?", subGraph)
	if err != nil {
		t.Fatalf("AskKnowledgeGraph failed: %v", err)
	}

	if ans != expectedAnswer {
		t.Fatalf("expected %q, got %q", expectedAnswer, ans)
	}

	_, err = llm.AskKnowledgeGraph(ctx, "   ", subGraph)
	if err == nil {
		t.Fatalf("expected error for empty question")
	}
}

func TestQueryGraph_EndToEndWithMocks(t *testing.T) {
	svc, q, userID, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	mockEmbClient := &http.Client{
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
	embSvc.SetHTTPClient(mockEmbClient)
	svc.embedding = embSvc

	expectedSynthesizedAnswer := "Segun el grafo de conocimiento, el diseno modular permite desacoplar los modulos de seguridad."

	mockLLMClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: expectedSynthesizedAnswer,
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

	llmSvc := NewLLMService("http://mock-llm.local", "test-key")
	llmSvc.SetHTTPClient(mockLLMClient)
	svc.llm = llmSvc

	seedIdea := createTestIdea(ctx, t, q, userID, "Diseno de Software Modular", "Principios de bajo acoplamiento.")
	hop1Idea := createTestIdea(ctx, t, q, userID, "Autenticacion Segura", "Control de sesiones y tokens.")
	hop2Idea := createTestIdea(ctx, t, q, userID, "Rotacion de Refresh Tokens", "Revocacion y familias de tokens.")

	vecSeed, _ := SerializeVector([]float64{0.99, 0.01, 0.0})
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  seedIdea.ID,
		RestructuredContent: "Principios de bajo acoplamiento e interfaces.",
		Embedding:           vecSeed,
		Status:              "procesada",
	})

	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, seedIdea.ID, []string{"modular", "arquitectura"})
	linkTestIdeas(ctx, t, q, seedIdea.ID, hop1Idea.ID, "semantic", 0.91)
	linkTestIdeas(ctx, t, q, hop1Idea.ID, hop2Idea.ID, "concept_overlap", 0.84)

	res, err := svc.QueryGraph(ctx, "como estructurar el software modular?")
	if err != nil {
		t.Fatalf("QueryGraph failed: %v", err)
	}

	if res.Answer != expectedSynthesizedAnswer {
		t.Errorf("expected answer %q, got %q", expectedSynthesizedAnswer, res.Answer)
	}

	if len(res.Seeds) == 0 || res.Seeds[0].IdeaID != seedIdea.ID {
		t.Errorf("expected seedIdea to be primary seed")
	}

	if len(res.SubGraph.Nodes) != 3 {
		t.Errorf("expected 3 nodes in subgraph (seed, hop1, hop2), got %d", len(res.SubGraph.Nodes))
	}

	if !strings.Contains(res.ContextPrompt, "Contexto primario (Nodos Semilla):") {
		t.Errorf("expected ContextPrompt to include primary context section")
	}
	if !strings.Contains(res.ContextPrompt, "Contexto asociativo conectado (Vecinos directos y secundarios):") {
		t.Errorf("expected ContextPrompt to include associative context section")
	}

	_, err = svc.QueryGraph(ctx, "   ")
	if err == nil {
		t.Errorf("expected error for empty query")
	}
}
