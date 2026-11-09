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

func TestQueryExpansion_SuccessAndFallback(t *testing.T) {
	ctx := context.Background()

	expandedResult := "Denuncia de acoso sexual u hostigamiento docente. Medidas de proteccion y confidencialidad."

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			var req chatCompletionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				return nil, err
			}
			if len(req.Messages) < 2 {
				t.Fatalf("expected at least 2 messages in expansion request")
			}
			if !strings.Contains(req.Messages[0].Content, "normalización y expansión de consultas") {
				t.Errorf("expected system instruction to mention normalization and expansion")
			}
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: expandedResult,
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

	colloquial := "Oye si un profe me anda acosando por mensajes con quien voy?"
	res, err := llm.ExpandQuery(ctx, colloquial)
	if err != nil {
		t.Fatalf("ExpandQuery failed: %v", err)
	}
	if res != expandedResult {
		t.Errorf("expected %q, got %q", expandedResult, res)
	}

	_, err = llm.ExpandQuery(ctx, "   ")
	if err == nil {
		t.Fatalf("expected error for empty query")
	}
}

func TestAskKnowledgeGraph_ZeroNodes_ReturnsInsufficientContext(t *testing.T) {
	ctx := context.Background()

	llm := NewLLMService("http://mock-llm.local", "key")
	called := false
	llm.SetHTTPClient(&http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
			}, nil
		}),
	})

	emptySubGraph := &TraversalSubGraph{
		Nodes: []TraversalNode{},
	}

	ans, err := llm.AskKnowledgeGraph(ctx, "Cual es el protocolo?", emptySubGraph)
	if err != nil {
		t.Fatalf("AskKnowledgeGraph failed: %v", err)
	}

	if called {
		t.Errorf("LLM should not be called when subgraph has 0 nodes")
	}

	if ans != InsufficientContextResponse {
		t.Errorf("expected %q, got %q", InsufficientContextResponse, ans)
	}

	ansNil, err := llm.AskKnowledgeGraph(ctx, "Cual es el protocolo?", nil)
	if err != nil {
		t.Fatalf("AskKnowledgeGraph with nil failed: %v", err)
	}
	if ansNil != InsufficientContextResponse {
		t.Errorf("expected %q for nil subgraph, got %q", InsufficientContextResponse, ansNil)
	}
}

func TestAskKnowledgeGraph_PromptHardeningAgainstLeakage(t *testing.T) {
	ctx := context.Background()

	var capturedReq chatCompletionRequest

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			_ = json.NewDecoder(r.Body).Decode(&capturedReq)
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: "La denuncia debe presentarse ante la OCGD con clave de expediente reservado.",
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

	subGraph := &TraversalSubGraph{
		Nodes: []TraversalNode{
			{
				ID:          uuid.New(),
				Title:       "Protocolo contra Hostigamiento",
				Content:     "Se habilita clave de expediente reservado.",
				HopDistance: 0,
			},
		},
	}

	_, err := llm.AskKnowledgeGraph(ctx, "¿Como se protege mi identidad?", subGraph)
	if err != nil {
		t.Fatalf("AskKnowledgeGraph failed: %v", err)
	}

	if len(capturedReq.Messages) < 2 {
		t.Fatalf("expected at least 2 messages in LLM request")
	}

	systemMsg := capturedReq.Messages[0].Content
	userMsg := capturedReq.Messages[1].Content

	if !strings.Contains(systemMsg, "Grounding estricto") {
		t.Errorf("system prompt must enforce Grounding estricto")
	}
	if !strings.Contains(systemMsg, "Neutralidad de interfaz") {
		t.Errorf("system prompt must enforce Neutralidad de interfaz")
	}
	if !strings.Contains(systemMsg, "Está terminantemente prohibido usar frases como") {
		t.Errorf("system prompt must explicitly forbid internal architecture phrases")
	}
	if strings.Contains(userMsg, "Contexto del grafo de conocimiento:") {
		t.Errorf("user message must not label reference text as 'Contexto del grafo de conocimiento'")
	}
	if !strings.Contains(userMsg, "Documentación de referencia:") {
		t.Errorf("user message must use neutral 'Documentación de referencia'")
	}
}

func TestSeedRetrieval_StrictThresholdPruning(t *testing.T) {
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

	vecIrrelevant, _ := SerializeVector([]float64{0.10, 0.99, 0.0})
	irrelevantIdea := createTestIdea(ctx, t, q, userID, "Tramite de Revision de Examenes", "Comision designada para revisar examen escrito")

	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  irrelevantIdea.ID,
		RestructuredContent: irrelevantIdea.Content,
		Embedding:           vecIrrelevant,
		Status:              "procesada",
	})

	seeds, err := svc.FindSeedIdeas(ctx, "acoso hostigamiento denuncia confidencial", 3)
	if err != nil {
		t.Fatalf("FindSeedIdeas failed: %v", err)
	}

	if len(seeds) != 0 {
		t.Fatalf("expected 0 seeds due to strict threshold pruning, got %d (top score: %f)", len(seeds), seeds[0].Score)
	}
}

func TestQueryGraph_ColloquialHarassmentRetrieval_EndToEnd(t *testing.T) {
	svc, q, userID, cleanup := setupTestSeedService(t)
	defer cleanup()

	ctx := context.Background()

	mockEmbClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{
					{Embedding: []float64{0.95, 0.31, 0.0}},
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

	expandedQuery := "Denuncia de hostigamiento o acoso docente por mensajes. Proteccion y confidencialidad ante la OCGD."
	expectedGroundedAnswer := "Debes presentar tu denuncia ante la Oficina Central de Genero y Diversidad (OCGD). Se te asignara un expediente reservado y podras solicitar medidas cautelares de proteccion."

	mockLLMClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			var chatReq chatCompletionRequest
			_ = json.NewDecoder(r.Body).Decode(&chatReq)

			content := expectedGroundedAnswer
			if len(chatReq.Messages) > 0 && strings.Contains(chatReq.Messages[0].Content, "normalización y expansión") {
				content = expandedQuery
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
	llmSvc := NewLLMService("http://mock-llm.local", "test-key")
	llmSvc.SetHTTPClient(mockLLMClient)
	svc.llm = llmSvc

	harassmentIdea := createTestIdea(ctx, t, q, userID, "Protocolo de Atencion a Denuncias de Acoso", "Oficina Central de Genero y Diversidad OCGD protege la identidad mediante expediente reservado.")
	vecHarassment, _ := SerializeVector([]float64{0.92, 0.38, 0.0})
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  harassmentIdea.ID,
		RestructuredContent: harassmentIdea.Content,
		Embedding:           vecHarassment,
		Status:              "procesada",
	})
	_, _ = svc.keywordService.SaveKeywordsForIdea(ctx, harassmentIdea.ID, []string{"acoso", "denuncia", "ocgd", "hostigamiento"})

	examIdea := createTestIdea(ctx, t, q, userID, "Revision de Examenes Academicos", "Comision designada para revision de pruebas.")
	vecExam, _ := SerializeVector([]float64{0.05, 0.99, 0.0})
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  examIdea.ID,
		RestructuredContent: examIdea.Content,
		Embedding:           vecExam,
		Status:              "procesada",
	})

	colloquialQuery := "Oye, si un profe me anda acosando por mensajes o tirando la onda, ¿con quien voy para que no tome represalias y que pasa si no quiero que se entere mi familia ni que el profe sepa que fui yo?"
	result, err := svc.QueryGraph(ctx, colloquialQuery)
	if err != nil {
		t.Fatalf("QueryGraph failed: %v", err)
	}

	if len(result.Seeds) == 0 {
		t.Fatalf("expected at least 1 seed for harassment query")
	}
	if result.Seeds[0].IdeaID != harassmentIdea.ID {
		t.Errorf("expected harassmentIdea as top seed, got %s (%s)", result.Seeds[0].IdeaID, result.Seeds[0].Title)
	}

	for _, s := range result.Seeds {
		if s.IdeaID == examIdea.ID {
			t.Errorf("exam revision idea should have been pruned by similarity threshold")
		}
	}

	if result.Answer != expectedGroundedAnswer {
		t.Errorf("expected grounded answer %q, got %q", expectedGroundedAnswer, result.Answer)
	}
}
