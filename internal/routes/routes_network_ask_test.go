package routes

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"
	"thinker/internal/middleware"
	"thinker/internal/services"
)

func TestNetworkAskHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chdirToProjectRoot(t)

	dsn := fmt.Sprintf("file:routes_network_ask_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed opening sqlite: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	if err := database.InitSchema(ctx, conn); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	queries := database.New(conn)
	llm := services.NewLLMService("http://127.0.0.1:8081", "")
	embed := services.NewEmbeddingService("http://127.0.0.1:8082", "")

	secret := []byte("0123456789abcdef0123456789abcdef")
	cfg := &config.BaseConfig{
		Debug:         true,
		AppBaseURL:    "http://127.0.0.1:8080",
		JWTSecret:     secret,
		SecureCookies: false,
	}

	router := SetUpRouter(queries, llm, embed, cfg)

	reqGetReg := httptest.NewRequest(http.MethodGet, "/auth/register", nil)
	wGetReg := httptest.NewRecorder()
	router.ServeHTTP(wGetReg, reqGetReg)
	csrfCookie := extractCookie(wGetReg.Result().Cookies(), middleware.CSRFCookieName(false))
	if csrfCookie == nil {
		t.Fatalf("missing csrf cookie")
	}

	reqUnauth := httptest.NewRequest(http.MethodPost, "/api/network/ask", bytes.NewReader([]byte(`{"question":"test"}`)))
	reqUnauth.Header.Set("Content-Type", "application/json")
	reqUnauth.Header.Set("X-CSRF-Token", csrfCookie.Value)
	reqUnauth.AddCookie(csrfCookie)
	wUnauth := httptest.NewRecorder()
	router.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", wUnauth.Code)
	}

	userEmail := "ada.lovelace@example.com"
	regParams := url.Values{
		"email":            {userEmail},
		"password":         {"AdaLovelace#2026Secure"},
		"password_confirm": {"AdaLovelace#2026Secure"},
	}

	reqReg := httptest.NewRequest(http.MethodPost, "/auth/register", stringsReader(regParams.Encode()))
	reqReg.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReg.Header.Set("X-CSRF-Token", csrfCookie.Value)
	reqReg.AddCookie(csrfCookie)
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)

	createdUser, err := queries.GetUserByEmail(ctx, userEmail)
	if err != nil {
		t.Fatalf("failed fetching created user: %v", err)
	}
	_, _ = queries.MarkUserEmailVerified(ctx, createdUser.ID)

	loginParams := url.Values{
		"email":    {userEmail},
		"password": {"AdaLovelace#2026Secure"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/auth/login", stringsReader(loginParams.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("X-CSRF-Token", csrfCookie.Value)
	reqLogin.AddCookie(csrfCookie)
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)

	accessCookie := extractCookie(wLogin.Result().Cookies(), middleware.AccessCookieName(false))
	if accessCookie == nil {
		t.Fatalf("missing access token cookie on login")
	}

	ideaID := uuid.New()
	_, err = queries.CreateIdea(ctx, database.CreateIdeaParams{
		ID:         ideaID,
		UserID:     createdUser.ID,
		GroupID:    ideaID,
		ChunkIndex: 1,
		ChunkTotal: 1,
		Title:      "Estructuras de Redes Neuronales",
		Content:    "Modelos de grafos y procesamiento distribuido.",
		Status:     "procesada",
	})
	if err != nil {
		t.Fatalf("failed creating test idea: %v", err)
	}

	askBody := []byte(`{"question": "como funcionan los grafos y redes neuronales?"}`)
	reqAsk := httptest.NewRequest(http.MethodPost, "/api/network/ask", bytes.NewReader(askBody))
	reqAsk.Header.Set("Content-Type", "application/json")
	reqAsk.Header.Set("X-CSRF-Token", csrfCookie.Value)
	reqAsk.AddCookie(csrfCookie)
	reqAsk.AddCookie(accessCookie)
	wAsk := httptest.NewRecorder()
	router.ServeHTTP(wAsk, reqAsk)

	if wAsk.Code != http.StatusOK {
		t.Fatalf("POST /api/network/ask failed with %d: %s", wAsk.Code, wAsk.Body.String())
	}

	var resp handlers.AskNetworkResponse
	if err := json.Unmarshal(wAsk.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed decoding ask response: %v", err)
	}

	if resp.Question != "como funcionan los grafos y redes neuronales?" {
		t.Errorf("expected echoed question, got %q", resp.Question)
	}

	if resp.Answer == "" {
		t.Errorf("expected non-empty answer")
	}

	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/network/ask", bytes.NewReader([]byte(`{"question": "   "}`)))
	reqEmpty.Header.Set("Content-Type", "application/json")
	reqEmpty.Header.Set("X-CSRF-Token", csrfCookie.Value)
	reqEmpty.AddCookie(csrfCookie)
	reqEmpty.AddCookie(accessCookie)
	wEmpty := httptest.NewRecorder()
	router.ServeHTTP(wEmpty, reqEmpty)

	if wEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty question, got %d", wEmpty.Code)
	}
}

func stringsReader(s string) *bytes.Reader {
	return bytes.NewReader([]byte(s))
}
