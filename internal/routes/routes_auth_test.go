package routes

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/middleware"
	"thinker/internal/services"
)

func chdirToProjectRoot(t *testing.T) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed resolving caller path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed changing directory to root %s: %v", root, err)
	}
}

func extractCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestEndToEndAuthHTTPFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chdirToProjectRoot(t)

	dsn := fmt.Sprintf("file:routes_auth_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
	cfg := &config.BaseConfig{
		Debug:         true,
		AppBaseURL:    "http://127.0.0.1:8080",
		JWTSecret:     []byte("0123456789abcdef0123456789abcdef"),
		SecureCookies: false,
	}

	router := SetUpRouter(queries, llm, embed, cfg)

	reqUnauth := httptest.NewRequest(http.MethodGet, "/web/ideas/list", nil)
	wUnauth := httptest.NewRecorder()
	router.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusSeeOther || wUnauth.Header().Get("Location") != "/auth/login" {
		t.Fatalf("expected 303 redirect to /auth/login, got status=%d loc=%q", wUnauth.Code, wUnauth.Header().Get("Location"))
	}

	reqAPIUnauth := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	wAPIUnauth := httptest.NewRecorder()
	router.ServeHTTP(wAPIUnauth, reqAPIUnauth)
	if wAPIUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on /api/users, got %d", wAPIUnauth.Code)
	}

	reqGetReg := httptest.NewRequest(http.MethodGet, "/auth/register", nil)
	wGetReg := httptest.NewRecorder()
	router.ServeHTTP(wGetReg, reqGetReg)
	if wGetReg.Code != http.StatusOK {
		t.Fatalf("GET /auth/register failed with %d", wGetReg.Code)
	}
	csrfCookie := extractCookie(wGetReg.Result().Cookies(), middleware.CSRFCookieName(false))
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatalf("expected CSRF cookie on GET /auth/register")
	}

	formNoCSRF := url.Values{}
	formNoCSRF.Set("email", "alan.turing@example.com")
	formNoCSRF.Set("password", "EnigmaMachine!1939")
	formNoCSRF.Set("password_confirm", "EnigmaMachine!1939")
	reqRegNoCSRF := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(formNoCSRF.Encode()))
	reqRegNoCSRF.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRegNoCSRF.AddCookie(csrfCookie)
	wRegNoCSRF := httptest.NewRecorder()
	router.ServeHTTP(wRegNoCSRF, reqRegNoCSRF)
	if wRegNoCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when CSRF token is missing, got %d", wRegNoCSRF.Code)
	}

	formReg := url.Values{}
	formReg.Set("csrf_token", csrfCookie.Value)
	formReg.Set("email", "  Alan.Turing@Example.COM ")
	formReg.Set("password", "EnigmaMachine!1939")
	formReg.Set("password_confirm", "EnigmaMachine!1939")
	reqReg := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(formReg.Encode()))
	reqReg.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReg.AddCookie(csrfCookie)
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on registration, got %d body=%s", wReg.Code, wReg.Body.String())
	}

	formLogin := url.Values{}
	formLogin.Set("csrf_token", csrfCookie.Value)
	formLogin.Set("email", "alan.turing@example.com")
	formLogin.Set("password", "EnigmaMachine!1939")
	reqLoginUnverified := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(formLogin.Encode()))
	reqLoginUnverified.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLoginUnverified.AddCookie(csrfCookie)
	wLoginUnverified := httptest.NewRecorder()
	router.ServeHTTP(wLoginUnverified, reqLoginUnverified)
	if wLoginUnverified.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unverified email login, got %d", wLoginUnverified.Code)
	}

	user, err := queries.GetUserByEmail(ctx, "alan.turing@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if _, err := queries.MarkUserEmailVerified(ctx, user.ID); err != nil {
		t.Fatalf("MarkUserEmailVerified failed: %v", err)
	}

	reqLogin := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(formLogin.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.AddCookie(csrfCookie)
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusSeeOther || wLogin.Header().Get("Location") != "/web/ideas/list" {
		t.Fatalf("expected 303 redirect to /web/ideas/list after login, got %d", wLogin.Code)
	}

	accessCookie := extractCookie(wLogin.Result().Cookies(), middleware.AccessCookieName(false))
	refreshCookie := extractCookie(wLogin.Result().Cookies(), middleware.RefreshCookieName(false))
	if accessCookie == nil || refreshCookie == nil || !accessCookie.HttpOnly || !refreshCookie.HttpOnly {
		t.Fatalf("expected HttpOnly access and refresh cookies on login")
	}

	reqIdeas := httptest.NewRequest(http.MethodGet, "/web/ideas/list", nil)
	reqIdeas.AddCookie(accessCookie)
	reqIdeas.AddCookie(refreshCookie)
	wIdeas := httptest.NewRecorder()
	router.ServeHTTP(wIdeas, reqIdeas)
	if wIdeas.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /web/ideas/list with valid session, got %d", wIdeas.Code)
	}

	formLogoutAll := url.Values{}
	formLogoutAll.Set("csrf_token", csrfCookie.Value)
	reqLogoutAll := httptest.NewRequest(http.MethodPost, "/auth/logout-all", strings.NewReader(formLogoutAll.Encode()))
	reqLogoutAll.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogoutAll.AddCookie(csrfCookie)
	reqLogoutAll.AddCookie(accessCookie)
	wLogoutAll := httptest.NewRecorder()
	router.ServeHTTP(wLogoutAll, reqLogoutAll)
	if wLogoutAll.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 on /auth/logout-all, got %d", wLogoutAll.Code)
	}

	reqIdeasAfterLogout := httptest.NewRequest(http.MethodGet, "/web/ideas/list", nil)
	reqIdeasAfterLogout.AddCookie(accessCookie)
	reqIdeasAfterLogout.AddCookie(refreshCookie)
	wIdeasAfterLogout := httptest.NewRecorder()
	router.ServeHTTP(wIdeasAfterLogout, reqIdeasAfterLogout)
	if wIdeasAfterLogout.Code != http.StatusSeeOther || wIdeasAfterLogout.Header().Get("Location") != "/auth/login" {
		t.Fatalf("expected old cookies to be rejected after logout-all, got status=%d", wIdeasAfterLogout.Code)
	}
}
