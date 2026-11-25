package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func setupTestRBACEnv(t *testing.T) (*IdeaService, *RoleService, *UserService, *database.Queries, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:rbac_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
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
	ideaSvc := NewIdeaService(queries, nil, nil)
	roleSvc := NewRoleService(queries)
	roleSvc.SetOnIdeasAffected(ideaSvc.EnqueueIdeasRelink)
	userSvc := NewUserService(queries)

	cleanup := func() {
		_ = conn.Close()
	}

	return ideaSvc, roleSvc, userSvc, queries, cleanup
}

func createTestUserActor(t *testing.T, q *database.Queries, email string, isSuperuser bool) Actor {
	t.Helper()
	ctx := context.Background()
	role := "member"
	superVal := int32(0)
	if isSuperuser {
		role = "admin"
		superVal = 1
	}
	u, err := q.CreateUserWithSuperuser(ctx, database.CreateUserWithSuperuserParams{
		ID:            uuid.New(),
		Email:         email,
		PasswordHash:  "hash",
		Role:          role,
		IsSuperuser:   superVal,
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 1,
	})
	if err != nil {
		t.Fatalf("CreateUserWithSuperuser failed: %v", err)
	}
	return NewActorFromUser(u, nil)
}

func TestRBAC_RoleCRUDAndUserMultiRole(t *testing.T) {
	_, roleSvc, userSvc, q, cleanup := setupTestRBACEnv(t)
	defer cleanup()

	ctx := context.Background()
	super := createTestUserActor(t, q, "super@thinker.test", true)
	member := createTestUserActor(t, q, "member@thinker.test", false)

	if _, err := roleSvc.CreateRole(ctx, member, "Ventas", "Equipo comercial"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-superuser creating role, got %v", err)
	}

	engRole, err := roleSvc.CreateRole(ctx, super, "Ingeniería", "Desarrollo")
	if err != nil {
		t.Fatalf("CreateRole engineering failed: %v", err)
	}
	if engRole.Slug != "ingenieria" {
		t.Fatalf("expected slug ingenieria, got %s", engRole.Slug)
	}

	salesRole, err := roleSvc.CreateRole(ctx, super, "Ventas", "Comercial")
	if err != nil {
		t.Fatalf("CreateRole sales failed: %v", err)
	}

	if _, err := roleSvc.CreateRole(ctx, super, "ingenieria", "Duplicate"); !errors.Is(err, ErrRoleAlreadyExists) {
		t.Fatalf("expected ErrRoleAlreadyExists, got %v", err)
	}

	createdUser, err := userSvc.CreateUser(ctx, super, "multi@thinker.test", "SuperSecure#2026!", false, []uuid.UUID{engRole.ID, salesRole.ID})
	if err != nil {
		t.Fatalf("CreateUser multi-role failed: %v", err)
	}

	assigned, err := q.ListRolesByUserID(ctx, createdUser.ID)
	if err != nil || len(assigned) != 2 {
		t.Fatalf("expected 2 roles assigned, got %d err=%v", len(assigned), err)
	}

	if _, err := userSvc.UpdateUserAccess(ctx, super, super.UserID, false, nil); !errors.Is(err, ErrCannotSelfDemote) {
		t.Fatalf("expected ErrCannotSelfDemote, got %v", err)
	}

	secondSuper, err := userSvc.CreateUser(ctx, super, "super2@thinker.test", "SuperSecure#2026!", true, nil)
	if err != nil {
		t.Fatalf("CreateUser second superuser failed: %v", err)
	}
	if _, err := userSvc.UpdateUserAccess(ctx, super, secondSuper.ID, false, []uuid.UUID{salesRole.ID}); err != nil {
		t.Fatalf("demoting second superuser failed: %v", err)
	}
	demoted, err := q.GetUserByID(ctx, secondSuper.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if demoted.IsSuperuser != 0 || demoted.TokenVersion <= secondSuper.TokenVersion {
		t.Fatalf("expected demoted user to have is_superuser=0 and bumped token_version, got super=%d ver=%d", demoted.IsSuperuser, demoted.TokenVersion)
	}
}

func TestRBAC_IdeaPermissionsViewEditAuthorSuperuser(t *testing.T) {
	ideaSvc, roleSvc, userSvc, q, cleanup := setupTestRBACEnv(t)
	defer cleanup()

	ctx := context.Background()
	super := createTestUserActor(t, q, "super@thinker.test", true)
	author := createTestUserActor(t, q, "author@thinker.test", false)
	editor := createTestUserActor(t, q, "editor@thinker.test", false)
	viewer := createTestUserActor(t, q, "viewer@thinker.test", false)
	outsider := createTestUserActor(t, q, "outsider@thinker.test", false)

	engRole, err := roleSvc.CreateRole(ctx, super, "Ingenieria", "")
	if err != nil {
		t.Fatalf("CreateRole eng failed: %v", err)
	}
	salesRole, err := roleSvc.CreateRole(ctx, super, "Ventas", "")
	if err != nil {
		t.Fatalf("CreateRole sales failed: %v", err)
	}

	_, _ = userSvc.UpdateUserAccess(ctx, super, editor.UserID, false, []uuid.UUID{engRole.ID})
	_, _ = userSvc.UpdateUserAccess(ctx, super, viewer.UserID, false, []uuid.UUID{salesRole.ID})

	if _, err := ideaSvc.ListIdeas(ctx, Actor{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for empty actor, got %v", err)
	}

	created, err := ideaSvc.CreateIdeasFromInput(ctx, author, "Arquitectura Interna", "Contenido privado inicial.", nil)
	if err != nil || len(created) != 1 {
		t.Fatalf("CreateIdeasFromInput failed: %v", err)
	}
	ideaID := created[0].ID.String()

	if _, err := ideaSvc.GetIdeaDetail(ctx, outsider, ideaID); !errors.Is(err, ErrIdeaNotFound) {
		t.Fatalf("expected ErrIdeaNotFound for outsider on private note, got %v", err)
	}
	if _, err := ideaSvc.GetIdeaDetail(ctx, viewer, ideaID); !errors.Is(err, ErrIdeaNotFound) {
		t.Fatalf("expected ErrIdeaNotFound for viewer on private note, got %v", err)
	}
	if _, err := ideaSvc.GetIdeaDetail(ctx, super, ideaID); err != nil {
		t.Fatalf("expected superuser to access private note, got %v", err)
	}

	perms := map[uuid.UUID]string{
		engRole.ID:   PermissionEdit,
		salesRole.ID: PermissionView,
	}
	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, author, ideaID, perms); err != nil {
		t.Fatalf("author UpdateIdeaPermissions failed: %v", err)
	}

	viewerDetail, err := ideaSvc.GetIdeaDetail(ctx, viewer, ideaID)
	if err != nil {
		t.Fatalf("viewer GetIdeaDetail failed: %v", err)
	}
	if viewerDetail.CanEdit || viewerDetail.CanDelete || viewerDetail.CanManageRoles {
		t.Fatalf("viewer should have read-only capabilities: %+v", viewerDetail)
	}
	if _, err := ideaSvc.UpdateIdea(ctx, viewer, ideaID, "Hack", "Hack"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for viewer UpdateIdea, got %v", err)
	}
	if _, err := ideaSvc.ReanalyzeIdea(ctx, viewer, ideaID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for viewer ReanalyzeIdea, got %v", err)
	}
	if err := ideaSvc.DeleteIdeaFragment(ctx, viewer, ideaID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for viewer DeleteIdeaFragment, got %v", err)
	}
	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, viewer, ideaID, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for viewer UpdateIdeaPermissions, got %v", err)
	}

	editorDetail, err := ideaSvc.GetIdeaDetail(ctx, editor, ideaID)
	if err != nil {
		t.Fatalf("editor GetIdeaDetail failed: %v", err)
	}
	if !editorDetail.CanEdit || editorDetail.CanDelete || editorDetail.CanManageRoles {
		t.Fatalf("editor capabilities mismatch: edit=%v del=%v manage=%v", editorDetail.CanEdit, editorDetail.CanDelete, editorDetail.CanManageRoles)
	}
	updatedDetail, err := ideaSvc.UpdateIdea(ctx, editor, ideaID, "Arquitectura Actualizada", "Contenido editado por colaborador.")
	if err != nil {
		t.Fatalf("editor UpdateIdea failed: %v", err)
	}
	if updatedDetail.Idea.UserID != author.UserID {
		t.Fatalf("expected original author to be preserved, got %v", updatedDetail.Idea.UserID)
	}
	if !updatedDetail.Idea.UpdatedBy.Valid || updatedDetail.Idea.UpdatedBy.UUID != editor.UserID {
		t.Fatalf("expected updated_by=%v, got %+v", editor.UserID, updatedDetail.Idea.UpdatedBy)
	}
	if _, err := ideaSvc.ReanalyzeIdea(ctx, editor, ideaID); err != nil {
		t.Fatalf("editor ReanalyzeIdea failed: %v", err)
	}
	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, editor, ideaID, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for editor changing permissions, got %v", err)
	}
	if err := ideaSvc.DeleteIdeaFragment(ctx, editor, ideaID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for editor deleting note, got %v", err)
	}
	if err := ideaSvc.DeleteIdeaGroup(ctx, editor, ideaID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for editor deleting group, got %v", err)
	}

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, super, ideaID, map[uuid.UUID]string{salesRole.ID: PermissionView}); err != nil {
		t.Fatalf("superuser UpdateIdeaPermissions failed: %v", err)
	}
	if err := ideaSvc.DeleteIdeaFragment(ctx, super, ideaID); err != nil {
		t.Fatalf("superuser DeleteIdeaFragment failed: %v", err)
	}
}

func TestRBAC_GroupPermissionsSync(t *testing.T) {
	ideaSvc, roleSvc, _, q, cleanup := setupTestRBACEnv(t)
	defer cleanup()

	ctx := context.Background()
	super := createTestUserActor(t, q, "super@thinker.test", true)
	author := createTestUserActor(t, q, "author@thinker.test", false)

	roleA, err := roleSvc.CreateRole(ctx, super, "Producto", "")
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	longText := strings.Repeat("Este es un parrafo tecnico extenso sobre arquitectura modular y permisos. ", 80)
	created, err := ideaSvc.CreateIdeasFromInput(ctx, author, "Documento Extenso", longText, nil)
	if err != nil {
		t.Fatalf("CreateIdeasFromInput failed: %v", err)
	}
	if len(created) < 2 {
		t.Fatalf("expected multi-chunk group, got %d chunks", len(created))
	}

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, author, created[0].ID.String(), map[uuid.UUID]string{roleA.ID: PermissionView}); err != nil {
		t.Fatalf("UpdateIdeaPermissions failed: %v", err)
	}

	for _, frag := range created {
		perms, err := q.ListIdeaRolePermissionsByIdeaID(ctx, frag.ID)
		if err != nil {
			t.Fatalf("ListIdeaRolePermissionsByIdeaID failed: %v", err)
		}
		if len(perms) != 1 || perms[0].RoleID != roleA.ID || perms[0].Permission != PermissionView {
			t.Fatalf("expected fragment %s to inherit synced permission, got %+v", frag.ID, perms)
		}
	}
}

func TestRBAC_HybridScopedLinkingAndRelink(t *testing.T) {
	ideaSvc, roleSvc, _, q, cleanup := setupTestRBACEnv(t)
	defer cleanup()

	ctx := context.Background()
	super := createTestUserActor(t, q, "super@thinker.test", true)
	authorA := createTestUserActor(t, q, "a@thinker.test", false)
	authorB := createTestUserActor(t, q, "b@thinker.test", false)

	engRole, err := roleSvc.CreateRole(ctx, super, "Ingenieria", "")
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	vec, _ := SerializeVector([]float64{0.98, 0.20, 0.0})

	ideaA := createTestIdea(ctx, t, q, authorA.UserID, "Cifrado de Sesiones", "Manejo seguro de tokens JWT")
	ideaB := createTestIdea(ctx, t, q, authorB.UserID, "Rotacion de Claves", "Seguridad criptografica y tokens JWT")

	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaA.ID,
		RestructuredContent: ideaA.Content,
		Embedding:           vec,
		Status:              "procesada",
	})
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaB.ID,
		RestructuredContent: ideaB.Content,
		Embedding:           vec,
		Status:              "procesada",
	})
	_, _ = ideaSvc.keywordService.SaveKeywordsForIdea(ctx, ideaA.ID, []string{"seguridad", "jwt"})
	_, _ = ideaSvc.keywordService.SaveKeywordsForIdea(ctx, ideaB.ID, []string{"seguridad", "jwt"})

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, authorB, ideaB.ID.String(), map[uuid.UUID]string{engRole.ID: PermissionView}); err != nil {
		t.Fatalf("UpdateIdeaPermissions B failed: %v", err)
	}

	if err := ideaSvc.RelinkIdeaScope(ctx, ideaA.ID); err != nil {
		t.Fatalf("RelinkIdeaScope A failed: %v", err)
	}
	links, _ := q.ListAllIdeaLinks(ctx)
	if len(links) != 0 {
		t.Fatalf("expected 0 links between notes with no shared scope, got %d", len(links))
	}

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, authorA, ideaA.ID.String(), map[uuid.UUID]string{engRole.ID: PermissionView}); err != nil {
		t.Fatalf("UpdateIdeaPermissions A share failed: %v", err)
	}
	links, _ = q.ListAllIdeaLinks(ctx)
	if len(links) == 0 {
		t.Fatalf("expected links to be created once ideas share engRole")
	}

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, authorA, ideaA.ID.String(), map[uuid.UUID]string{}); err != nil {
		t.Fatalf("UpdateIdeaPermissions A unshare failed: %v", err)
	}
	links, _ = q.ListAllIdeaLinks(ctx)
	if len(links) != 0 {
		t.Fatalf("expected links to be removed after unsharing role, got %d", len(links))
	}
}

func TestRBAC_GraphAndHierarchicalRAGZeroLeak(t *testing.T) {
	ideaSvc, roleSvc, userSvc, q, cleanup := setupTestRBACEnv(t)
	defer cleanup()

	ctx := context.Background()
	super := createTestUserActor(t, q, "super@thinker.test", true)
	author := createTestUserActor(t, q, "author@thinker.test", false)
	viewer := createTestUserActor(t, q, "viewer@thinker.test", false)

	publicRole, err := roleSvc.CreateRole(ctx, super, "General", "")
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	_, _ = userSvc.UpdateUserAccess(ctx, super, viewer.UserID, false, []uuid.UUID{publicRole.ID})

	mocSecret, err := ideaSvc.topicService.CreateMOC(ctx, "MOC_CONFIDENCIAL", "Finanzas Secretas", "Salarios ejecutivos reservados", false)
	if err != nil {
		t.Fatalf("CreateMOC secret failed: %v", err)
	}
	hubSecret, err := ideaSvc.topicService.CreateHub(ctx, mocSecret.ID, "Nomina Directiva", "Compensaciones confidenciales", []string{"salarios", "bonos"}, false)
	if err != nil {
		t.Fatalf("CreateHub secret failed: %v", err)
	}

	mocPublic, err := ideaSvc.topicService.CreateMOC(ctx, "MOC_PUBLICO", "Guias Generales", "Documentacion abierta del equipo", false)
	if err != nil {
		t.Fatalf("CreateMOC public failed: %v", err)
	}
	hubPublic, err := ideaSvc.topicService.CreateHub(ctx, mocPublic.ID, "Onboarding", "Bienvenida y guias", []string{"onboarding", "guias"}, false)
	if err != nil {
		t.Fatalf("CreateHub public failed: %v", err)
	}

	vec, _ := SerializeVector([]float64{0.95, 0.15, 0.0})
	secretIdea := createTestIdea(ctx, t, q, author.UserID, "Salarios Confidenciales", "Tabla secreta de compensaciones")
	publicIdea := createTestIdea(ctx, t, q, author.UserID, "Guia de Onboarding", "Pasos iniciales para nuevos integrantes")

	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  secretIdea.ID,
		RestructuredContent: secretIdea.Content,
		Embedding:           vec,
		Status:              "procesada",
	})
	_, _ = q.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  publicIdea.ID,
		RestructuredContent: publicIdea.Content,
		Embedding:           vec,
		Status:              "procesada",
	})
	_, _ = ideaSvc.keywordService.SaveKeywordsForIdea(ctx, secretIdea.ID, []string{"salario-secreto"})
	_, _ = ideaSvc.keywordService.SaveKeywordsForIdea(ctx, publicIdea.ID, []string{"onboarding"})
	_ = ideaSvc.topicService.LinkIdeaToHub(ctx, hubSecret.ID, secretIdea.ID, "GOVERNS", false)
	_ = ideaSvc.topicService.LinkIdeaToHub(ctx, hubPublic.ID, publicIdea.ID, "GOVERNS", false)

	if _, err := ideaSvc.UpdateIdeaPermissions(ctx, author, publicIdea.ID.String(), map[uuid.UUID]string{publicRole.ID: PermissionView}); err != nil {
		t.Fatalf("UpdateIdeaPermissions publicIdea failed: %v", err)
	}

	linkTestIdeas(ctx, t, q, publicIdea.ID, secretIdea.ID, "semantic", 0.99)

	graph, err := ideaSvc.GetNetworkGraph(ctx, viewer)
	if err != nil {
		t.Fatalf("GetNetworkGraph failed: %v", err)
	}
	for _, n := range graph.Nodes {
		if n.ID == secretIdea.ID.String() || n.ID == "moc:"+mocSecret.ID.String() || n.ID == "hub:"+hubSecret.ID.String() || n.Label == "salario-secreto" {
			t.Fatalf("secret node leaked in viewer network graph: %+v", n)
		}
	}

	subGraph, err := ideaSvc.TraverseSubGraph(ctx, viewer, []uuid.UUID{publicIdea.ID}, DefaultTraversalOptions())
	if err != nil {
		t.Fatalf("TraverseSubGraph failed: %v", err)
	}
	for _, n := range subGraph.Nodes {
		if n.ID == secretIdea.ID {
			t.Fatalf("secret idea leaked via TraverseSubGraph neighbor expansion")
		}
	}

	var capturedTriagePrompt string
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			var req chatCompletionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, msg := range req.Messages {
				if strings.Contains(msg.Content, "MOC_") {
					capturedTriagePrompt += msg.Content
				}
			}
			content := `{"target_mocs": ["MOC_PUBLICO"]}`
			if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "Grounding estricto") {
				content = "Solo veo la guia de onboarding."
			}
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{Message: chatMessage{Role: "assistant", Content: content}},
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
	ideaSvc.llm = llm
	ideaSvc.topicService.llm = llm

	seeds, err := ideaSvc.FindHierarchicalSeeds(ctx, viewer, "salarios y guias de onboarding", 3)
	if err != nil {
		t.Fatalf("FindHierarchicalSeeds failed: %v", err)
	}
	if strings.Contains(capturedTriagePrompt, "MOC_CONFIDENCIAL") || strings.Contains(capturedTriagePrompt, "Finanzas Secretas") {
		t.Fatalf("private MOC leaked into LLM triage prompt: %s", capturedTriagePrompt)
	}
	for _, s := range seeds {
		if s.IdeaID == secretIdea.ID {
			t.Fatalf("secret idea leaked in hierarchical seeds")
		}
	}
}
