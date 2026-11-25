package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"thinker/internal/database"
)

var (
	ErrRoleNameRequired  = errors.New("el nombre del rol es obligatorio")
	ErrRoleAlreadyExists = errors.New("ya existe un rol con ese nombre o identificador")
	ErrRoleNotFound      = errors.New("rol no encontrado")
)

type RoleService struct {
	queries         *database.Queries
	onIdeasAffected func(ideaIDs []uuid.UUID)
}

func NewRoleService(queries *database.Queries) *RoleService {
	return &RoleService{queries: queries}
}

func (s *RoleService) SetOnIdeasAffected(fn func(ideaIDs []uuid.UUID)) {
	s.onIdeasAffected = fn
}

func (s *RoleService) ListRoles(ctx context.Context) ([]database.Role, error) {
	return s.queries.ListRoles(ctx)
}

func (s *RoleService) ListRolesWithUsage(ctx context.Context, actor Actor) ([]database.ListRolesWithUsageRow, error) {
	if !actor.CanManageUsersAndRoles() {
		return nil, ErrForbidden
	}
	return s.queries.ListRolesWithUsage(ctx)
}

func (s *RoleService) CreateRole(ctx context.Context, actor Actor, name, description string) (database.Role, error) {
	if !actor.CanManageUsersAndRoles() {
		return database.Role{}, ErrForbidden
	}

	cleanName := strings.TrimSpace(name)
	if len([]rune(cleanName)) < 2 || len([]rune(cleanName)) > 100 {
		return database.Role{}, ErrRoleNameRequired
	}

	slug := slugifyRoleName(cleanName)
	if slug == "" {
		return database.Role{}, ErrRoleNameRequired
	}

	if existing, err := s.queries.GetRoleBySlug(ctx, slug); err == nil && existing.ID != uuid.Nil {
		return database.Role{}, ErrRoleAlreadyExists
	}

	cleanDesc := strings.TrimSpace(description)
	if len([]rune(cleanDesc)) > 500 {
		cleanDesc = string([]rune(cleanDesc)[:500])
	}

	return s.queries.CreateRole(ctx, database.CreateRoleParams{
		ID:          uuid.New(),
		Name:        cleanName,
		Slug:        slug,
		Description: cleanDesc,
	})
}

func (s *RoleService) UpdateRole(ctx context.Context, actor Actor, roleID uuid.UUID, name, description string) (database.Role, error) {
	if !actor.CanManageUsersAndRoles() {
		return database.Role{}, ErrForbidden
	}

	if _, err := s.queries.GetRoleByID(ctx, roleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return database.Role{}, ErrRoleNotFound
		}
		return database.Role{}, err
	}

	cleanName := strings.TrimSpace(name)
	if len([]rune(cleanName)) < 2 || len([]rune(cleanName)) > 100 {
		return database.Role{}, ErrRoleNameRequired
	}

	slug := slugifyRoleName(cleanName)
	if slug == "" {
		return database.Role{}, ErrRoleNameRequired
	}

	if existing, err := s.queries.GetRoleBySlug(ctx, slug); err == nil && existing.ID != uuid.Nil && existing.ID != roleID {
		return database.Role{}, ErrRoleAlreadyExists
	}

	cleanDesc := strings.TrimSpace(description)
	if len([]rune(cleanDesc)) > 500 {
		cleanDesc = string([]rune(cleanDesc)[:500])
	}

	return s.queries.UpdateRole(ctx, database.UpdateRoleParams{
		ID:          roleID,
		Name:        cleanName,
		Slug:        slug,
		Description: cleanDesc,
	})
}

func (s *RoleService) DeleteRole(ctx context.Context, actor Actor, roleID uuid.UUID) error {
	if !actor.CanManageUsersAndRoles() {
		return ErrForbidden
	}

	if _, err := s.queries.GetRoleByID(ctx, roleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRoleNotFound
		}
		return err
	}

	affectedIdeaIDs, _ := s.queries.ListIdeaIDsByRoleID(ctx, roleID)

	if err := s.queries.DeleteRole(ctx, roleID); err != nil {
		return err
	}

	if s.onIdeasAffected != nil && len(affectedIdeaIDs) > 0 {
		s.onIdeasAffected(affectedIdeaIDs)
	}

	return nil
}

func slugifyRoleName(raw string) string {
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
	)
	normalized := strings.ToLower(replacer.Replace(strings.TrimSpace(raw)))
	var b strings.Builder
	lastDash := false
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if unicode.IsSpace(r) || r == '-' || r == '_' {
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > 100 {
		res = strings.Trim(res[:100], "-")
	}
	return res
}
