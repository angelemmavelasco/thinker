package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func registerVerifyAndLogin(t *testing.T, router *gin.Engine, q *database.Queries, csrfCookie *http.Cookie, email, password string) (database.User, []*http.Cookie) {
	t.Helper()
	ctx := context.Background()

	regForm := url.Values{
		"csrf_token":       {csrfCookie.Value},
		"email":            {email},
		"password":         {password},
		"password_confirm": {password},
	}
	reqReg := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(regForm.Encode()))
	reqReg.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReg.AddCookie(csrfCookie)
	wReg := httptest.NewRecorder()
	router.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusCreated {
		t.Fatalf("register %s failed: status=%d body=%s", email, wReg.Code, wReg.Body.String())
	}

	u, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail %s failed: %v", email, err)
	}
	u, err = q.MarkUserEmailVerified(ctx, u.ID)
	if err != nil {
		t.Fatalf("MarkUserEmailVerified %s failed: %v", email, err)
	}

	loginForm := url.Values{
		"csrf_token": {csrfCookie.Value},
		"email":      {email},
		"password":   {password},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginForm.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.AddCookie(csrfCookie)
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusSeeOther {
		t.Fatalf("login %s failed: status=%d body=%s", email, wLogin.Code, wLogin.Body.String())
	}

	accessCookie := extractCookie(wLogin.Result().Cookies(), middleware.AccessCookieName(false))
	refreshCookie := extractCookie(wLogin.Result().Cookies(), middleware.RefreshCookieName(false))
	if accessCookie == nil || refreshCookie == nil {
		t.Fatalf("missing session cookies for %s", email)
	}

	return u, []*http.Cookie{csrfCookie, accessCookie, refreshCookie}
}

func doAuthRequest(router *gin.Engine, method, target string, cookies []*http.Cookie, form url.Values, htmx bool) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestRoutesRBAC_EndToEndSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chdirToProjectRoot(t)

	dsn := fmt.Sprintf("file:routes_rbac_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
		Debug:          true,
		AppBaseURL:     "http://127.0.0.1:8080",
		JWTSecret:      []byte("0123456789abcdef0123456789abcdef"),
		SecureCookies:  false,
		SuperuserEmail: "root@thinker.test",
	}

	router := SetUpRouter(queries, llm, embed, cfg)

	wInit := httptest.NewRecorder()
	router.ServeHTTP(wInit, httptest.NewRequest(http.MethodGet, "/auth/register", nil))
	csrfCookie := extractCookie(wInit.Result().Cookies(), middleware.CSRFCookieName(false))
	if csrfCookie == nil {
		t.Fatalf("missing csrf cookie")
	}

	superUser, superCookies := registerVerifyAndLogin(t, router, queries, csrfCookie, "root@thinker.test", "RootPass#2026!")
	if superUser.IsSuperuser != 1 {
		t.Fatalf("expected SUPERUSER_EMAIL account to be bootstrapped as superuser")
	}

	authorUser, authorCookies := registerVerifyAndLogin(t, router, queries, csrfCookie, "author@thinker.test", "AuthorPass#2026!")
	editorUser, editorCookies := registerVerifyAndLogin(t, router, queries, csrfCookie, "editor@thinker.test", "EditorPass#2026!")
	viewerUser, viewerCookies := registerVerifyAndLogin(t, router, queries, csrfCookie, "viewer@thinker.test", "ViewerPass#2026!")
	_, outsiderCookies := registerVerifyAndLogin(t, router, queries, csrfCookie, "outsider@thinker.test", "OutsiderPass#2026!")

	if authorUser.IsSuperuser != 0 || editorUser.IsSuperuser != 0 || viewerUser.IsSuperuser != 0 {
		t.Fatalf("publicly registered users must never receive superuser privileges")
	}

	for _, endpoint := range []string{"/api/users", "/web/users", "/web/users/new", "/web/roles"} {
		w := doAuthRequest(router, http.MethodGet, endpoint, authorCookies, nil, false)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for non-superuser GET %s, got %d", endpoint, w.Code)
		}
	}

	for _, endpoint := range []string{"/web/roles", "/web/topics/mocs", "/web/topics/settings/auto-mocs"} {
		w := doAuthRequest(router, http.MethodPost, endpoint, authorCookies, url.Values{"csrf_token": {csrfCookie.Value}, "name": {"Hack"}, "title": {"Hack"}}, false)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for non-superuser POST %s, got %d", endpoint, w.Code)
		}
	}

	wNoCSRF := doAuthRequest(router, http.MethodPost, "/web/roles", superCookies, url.Values{"name": {"Ingenieria"}}, true)
	if wNoCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when CSRF token is missing on POST /web/roles, got %d", wNoCSRF.Code)
	}

	wCreateEng := doAuthRequest(router, http.MethodPost, "/web/roles", superCookies, url.Values{
		"csrf_token":  {csrfCookie.Value},
		"name":        {"Ingenieria"},
		"description": {"<script>alert(1)</script>"},
	}, true)
	if wCreateEng.Code != http.StatusOK {
		t.Fatalf("superuser creating role Ingenieria failed: %d %s", wCreateEng.Code, wCreateEng.Body.String())
	}
	if strings.Contains(wCreateEng.Body.String(), "<script>alert(1)</script>") {
		t.Fatalf("unescaped XSS payload detected in roles HTML response")
	}

	wCreateSales := doAuthRequest(router, http.MethodPost, "/web/roles", superCookies, url.Values{
		"csrf_token":  {csrfCookie.Value},
		"name":        {"Ventas"},
		"description": {"Comercial"},
	}, true)
	if wCreateSales.Code != http.StatusOK {
		t.Fatalf("superuser creating role Ventas failed: %d", wCreateSales.Code)
	}

	engRole, err := queries.GetRoleBySlug(ctx, "ingenieria")
	if err != nil {
		t.Fatalf("GetRoleBySlug ingenieria failed: %v", err)
	}
	salesRole, err := queries.GetRoleBySlug(ctx, "ventas")
	if err != nil {
		t.Fatalf("GetRoleBySlug ventas failed: %v", err)
	}

	wAssignEditor := doAuthRequest(router, http.MethodPost, "/web/users/"+editorUser.ID.String()+"/access", superCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
		"role_ids":   {engRole.ID.String()},
	}, true)
	if wAssignEditor.Code != http.StatusOK {
		t.Fatalf("assigning engRole to editor failed: %d %s", wAssignEditor.Code, wAssignEditor.Body.String())
	}

	wAssignViewer := doAuthRequest(router, http.MethodPost, "/web/users/"+viewerUser.ID.String()+"/access", superCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
		"role_ids":   {salesRole.ID.String()},
	}, true)
	if wAssignViewer.Code != http.StatusOK {
		t.Fatalf("assigning salesRole to viewer failed: %d", wAssignViewer.Code)
	}

	wCreateIdea := doAuthRequest(router, http.MethodPost, "/web/ideas/new", authorCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
		"title":      {"Estrategia Q4"},
		"content":    {"Plan confidencial de lanzamientos"},
	}, false)
	if wCreateIdea.Code != http.StatusSeeOther {
		t.Fatalf("author CreateIdea failed: %d %s", wCreateIdea.Code, wCreateIdea.Body.String())
	}

	allIdeas, err := queries.ListIdeasByUserID(ctx, authorUser.ID)
	if err != nil || len(allIdeas) != 1 {
		t.Fatalf("expected 1 idea by author, got %d err=%v", len(allIdeas), err)
	}
	ideaID := allIdeas[0].ID.String()

	if w := doAuthRequest(router, http.MethodGet, "/web/ideas/"+ideaID, outsiderCookies, nil, false); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 IDOR protection for outsider, got %d", w.Code)
	}
	if w := doAuthRequest(router, http.MethodGet, "/web/ideas/"+ideaID, viewerCookies, nil, false); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for viewer before sharing, got %d", w.Code)
	}

	wShare := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/permissions", authorCookies, url.Values{
		"csrf_token":                        {csrfCookie.Value},
		"role_perm[" + engRole.ID.String() + "]":   {"edit"},
		"role_perm[" + salesRole.ID.String() + "]": {"view"},
	}, true)
	if wShare.Code != http.StatusOK {
		t.Fatalf("author sharing idea failed: %d %s", wShare.Code, wShare.Body.String())
	}

	wViewerDetail := doAuthRequest(router, http.MethodGet, "/web/ideas/"+ideaID, viewerCookies, nil, true)
	if wViewerDetail.Code != http.StatusOK {
		t.Fatalf("expected viewer to view shared idea, got %d", wViewerDetail.Code)
	}
	if strings.Contains(wViewerDetail.Body.String(), "Editar idea") || strings.Contains(wViewerDetail.Body.String(), "idea-permissions-form") {
		t.Fatalf("viewer HTML must not expose edit or permissions controls")
	}

	if w := doAuthRequest(router, http.MethodGet, "/web/ideas/"+ideaID+"/edit", viewerCookies, nil, true); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer GET edit, got %d", w.Code)
	}
	if w := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/edit", viewerCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
		"title":      {"Tampered"},
		"content":    {"Tampered"},
	}, true); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer POST edit, got %d", w.Code)
	}

	wEditorEdit := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/edit", editorCookies, url.Values{
		"csrf_token":                               {csrfCookie.Value},
		"title":                                    {"Estrategia Q4 Revisada"},
		"content":                                  {"Contenido actualizado por ingenieria"},
		"role_perm[" + salesRole.ID.String() + "]": {""},
	}, true)
	if wEditorEdit.Code != http.StatusOK {
		t.Fatalf("expected editor POST edit to succeed, got %d %s", wEditorEdit.Code, wEditorEdit.Body.String())
	}

	permsAfterEdit, _ := queries.ListIdeaRolePermissionsByIdeaID(ctx, allIdeas[0].ID)
	if len(permsAfterEdit) != 2 {
		t.Fatalf("editor POST edit must not alter role permissions, got %d", len(permsAfterEdit))
	}

	if w := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/permissions", editorCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
	}, true); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for editor changing permissions, got %d", w.Code)
	}
	if w := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/delete", editorCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
	}, true); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for editor deleting idea, got %d", w.Code)
	}

	wGraphViewer := doAuthRequest(router, http.MethodGet, "/api/network/graph", viewerCookies, nil, false)
	if wGraphViewer.Code != http.StatusOK {
		t.Fatalf("viewer network graph failed: %d", wGraphViewer.Code)
	}
	var graphViewer services.GraphData
	if err := json.Unmarshal(wGraphViewer.Body.Bytes(), &graphViewer); err != nil {
		t.Fatalf("unmarshal viewer graph failed: %v", err)
	}
	foundInViewerGraph := false
	for _, n := range graphViewer.Nodes {
		if n.ID == ideaID {
			foundInViewerGraph = true
		}
	}
	if !foundInViewerGraph {
		t.Fatalf("expected shared idea in viewer graph")
	}

	wRevoke := doAuthRequest(router, http.MethodPost, "/web/users/"+viewerUser.ID.String()+"/access", superCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
	}, true)
	if wRevoke.Code != http.StatusOK {
		t.Fatalf("revoking viewer roles failed: %d", wRevoke.Code)
	}

	if w := doAuthRequest(router, http.MethodGet, "/web/ideas/"+ideaID, viewerCookies, nil, true); w.Code != http.StatusNotFound {
		t.Fatalf("expected immediate 404 after revoking viewer role with same JWT, got %d", w.Code)
	}

	wTopicsViewer := doAuthRequest(router, http.MethodGet, "/web/topics", viewerCookies, nil, false)
	if wTopicsViewer.Code != http.StatusOK {
		t.Fatalf("GET /web/topics for viewer failed: %d", wTopicsViewer.Code)
	}
	if strings.Contains(wTopicsViewer.Body.String(), "Nuevo tema principal") {
		t.Fatalf("regular user must not see superuser topic management controls")
	}

	wSuperDelete := doAuthRequest(router, http.MethodPost, "/web/ideas/"+ideaID+"/delete", superCookies, url.Values{
		"csrf_token": {csrfCookie.Value},
	}, true)
	if wSuperDelete.Code != http.StatusOK {
		t.Fatalf("expected superuser to delete author's idea, got %d %s", wSuperDelete.Code, wSuperDelete.Body.String())
	}
}
