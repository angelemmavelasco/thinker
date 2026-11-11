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

func setupTestTopicService(t *testing.T) (*TopicService, *database.Queries, uuid.UUID, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:topic_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
		Email:         fmt.Sprintf("topic_user_%d@test.com", time.Now().UnixNano()),
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

	topicSvc := NewTopicService(queries, nil, nil)

	cleanup := func() {
		_ = conn.Close()
	}

	return topicSvc, queries, testUserID, cleanup
}

func TestTriageQueryMOC_DeterministicValidation(t *testing.T) {
	ctx := context.Background()

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{
						Message: chatMessage{
							Role:    "assistant",
							Content: `{"target_mocs": ["MOC_FARMACOS", "MOC_DEVOLUCIONES", "MOC_INEXISTENTE"]}`,
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

	available := []MOCDescriptor{
		{Code: "MOC_CANALES", Title: "Canales de Venta", Description: "Giro de clientes"},
		{Code: "MOC_FARMACOS", Title: "Fármacos y Biológicos", Description: "Vacunas y cadena de frío"},
		{Code: "MOC_DEVOLUCIONES", Title: "Devoluciones", Description: "Rechazos y mermas"},
	}

	mocs, err := llm.TriageQueryMOC(ctx, "se descompuso el refrigerador de vacunas", available)
	if err != nil {
		t.Fatalf("TriageQueryMOC failed: %v", err)
	}

	if len(mocs) != 2 {
		t.Fatalf("expected 2 valid MOCs, got %d", len(mocs))
	}
	if mocs[0] != "MOC_FARMACOS" || mocs[1] != "MOC_DEVOLUCIONES" {
		t.Errorf("unexpected triage result: %v", mocs)
	}
}

func TestResolveAndLinkIdea_HubDeduplication(t *testing.T) {
	topicSvc, q, userID, cleanup := setupTestTopicService(t)
	defer cleanup()

	ctx := context.Background()

	// Vector 1: [0.99, 0.01, 0.0]
	// Vector 2: [0.98, 0.02, 0.0] (high similarity > 0.99)
	// Vector 3: [0.01, 0.99, 0.0] (low similarity < 0.10)
	var currentVec []float64

	mockEmbClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{
					{Embedding: currentVec},
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
	topicSvc.embedding = embSvc

	idea1 := createTestIdea(ctx, t, q, userID, "Cláusula 24", "Venta prohibida de dietas a estéticas")
	currentVec = []float64{0.99, 0.01, 0.0}

	analysis1 := &IdeaAnalysisResult{
		RestructuredNote: "Prohibición de venta de dietas de prescripción a estéticas",
		KeyEntities:      []string{"Estética", "Dieta"},
		TargetMOCCode:    "MOC_CANALES",
		TargetMOCTitle:   "Canales y Restricciones Sanitarias",
		TargetMOCDesc:    "Requisitos por tipo de giro comercial",
		ProposedHubTitle: "Restricciones por Giro Comercial",
		HubSummary:       "Estéticas y peluquerías no pueden comprar prescripción",
		SearchAliases:    []string{"estetica", "peluqueria"},
	}

	moc1, hub1, err := topicSvc.ResolveAndLinkIdea(ctx, idea1.ID, analysis1)
	if err != nil {
		t.Fatalf("ResolveAndLinkIdea 1 failed: %v", err)
	}
	if moc1 == nil || hub1 == nil {
		t.Fatalf("expected moc and hub to be created")
	}
	if moc1.Code != "MOC_CANALES" {
		t.Errorf("expected MOC_CANALES, got %s", moc1.Code)
	}
	if hub1.Title != "Restricciones por Giro Comercial" {
		t.Errorf("expected hub title, got %s", hub1.Title)
	}

	// Idea 2: same concept, slightly different phrasing and alias
	idea2 := createTestIdea(ctx, t, q, userID, "Cláusula 25", "Grooming no puede adquirir medicamentos controlados")
	currentVec = []float64{0.98, 0.02, 0.0}

	analysis2 := &IdeaAnalysisResult{
		RestructuredNote: "Prohibición de venta de medicamentos controlados a centros de grooming",
		KeyEntities:      []string{"Grooming", "Medicamentos"},
		TargetMOCCode:    "MOC_CANALES",
		ProposedHubTitle: "Restricciones Comerciales de Giros",
		HubSummary:       "Grooming sin MVZ no puede adquirir medicamentos",
		SearchAliases:    []string{"grooming", "boutique"},
	}

	moc2, hub2, err := topicSvc.ResolveAndLinkIdea(ctx, idea2.ID, analysis2)
	if err != nil {
		t.Fatalf("ResolveAndLinkIdea 2 failed: %v", err)
	}
	if moc2.ID != moc1.ID {
		t.Errorf("expected idea2 to reuse moc1, got %v vs %v", moc2.ID, moc1.ID)
	}
	if hub2.ID != hub1.ID {
		t.Errorf("expected idea2 to deduplicate and reuse hub1, got %v vs %v", hub2.ID, hub1.ID)
	}

	// Verify aliases were merged
	updatedHub, _ := topicSvc.GetHubByID(ctx, hub1.ID)
	if !containsStr(updatedHub.SearchAliases, "grooming") || !containsStr(updatedHub.SearchAliases, "estetica") {
		t.Errorf("expected merged search aliases, got: %s", updatedHub.SearchAliases)
	}

	// Verify both ideas are linked to hub1
	linkedIdeas, err := topicSvc.ListIdeasByHub(ctx, hub1.ID)
	if err != nil {
		t.Fatalf("ListIdeasByHub failed: %v", err)
	}
	if len(linkedIdeas) != 2 {
		t.Fatalf("expected 2 linked ideas to hub1, got %d", len(linkedIdeas))
	}

	// Idea 3: distant concept in same MOC -> should create a new distinct Hub
	idea3 := createTestIdea(ctx, t, q, userID, "Cláusula 40", "Contratos con Hospitales y Unidades Quirúrgicas")
	currentVec = []float64{0.01, 0.99, 0.0}

	analysis3 := &IdeaAnalysisResult{
		RestructuredNote: "Acreditación de licencias para quirófanos veterinarios",
		KeyEntities:      []string{"Hospital", "Quirófano"},
		TargetMOCCode:    "MOC_CANALES",
		ProposedHubTitle: "Acreditación de Hospitales y Quirófanos",
		HubSummary:       "Requisitos de licencia COFEPRIS para venta a quirófanos",
		SearchAliases:    []string{"hospital", "cirugia"},
	}

	_, hub3, err := topicSvc.ResolveAndLinkIdea(ctx, idea3.ID, analysis3)
	if err != nil {
		t.Fatalf("ResolveAndLinkIdea 3 failed: %v", err)
	}
	if hub3.ID == hub1.ID {
		t.Errorf("expected distinct hub for distant concept, got same hub1 id")
	}
}

func TestTopicService_ManualOverrideAndMove(t *testing.T) {
	topicSvc, q, userID, cleanup := setupTestTopicService(t)
	defer cleanup()

	ctx := context.Background()

	mocA, _ := topicSvc.CreateMOC(ctx, "MOC_ALIMENTOS", "Alimentos", "Dietas", false)
	hubA, _ := topicSvc.CreateHub(ctx, mocA.ID, "Dietas Prescripción", "Resumen A", nil, false)

	mocB, _ := topicSvc.CreateMOC(ctx, "MOC_FARMACOS", "Fármacos", "Biológicos", false)
	hubB, _ := topicSvc.CreateHub(ctx, mocB.ID, "Biológicos", "Resumen B", nil, false)

	idea := createTestIdea(ctx, t, q, userID, "Cláusula 10", "Contenido X")

	// Initially linked to hubA
	_ = topicSvc.LinkIdeaToHub(ctx, hubA.ID, idea.ID, "GOVERNS", false)

	// User moves idea to hubB
	err := topicSvc.MoveIdeaToHub(ctx, idea.ID, mocB.ID, hubB.ID)
	if err != nil {
		t.Fatalf("MoveIdeaToHub failed: %v", err)
	}

	hubs, err := topicSvc.ListHubsByIdea(ctx, idea.ID)
	if err != nil {
		t.Fatalf("ListHubsByIdea failed: %v", err)
	}
	if len(hubs) != 1 {
		t.Fatalf("expected 1 hub after move, got %d", len(hubs))
	}
	if hubs[0].ID != hubB.ID {
		t.Errorf("expected hubB, got %v", hubs[0].ID)
	}
	if hubs[0].IsManualOverride != 1 {
		t.Errorf("expected is_manual_override to be 1")
	}
}

func containsStr(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
