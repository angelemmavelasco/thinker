package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"thinker/internal/database"
)

const (
	PermissionView = "view"
	PermissionEdit = "edit"
)

var (
	ErrUnauthorized      = errors.New("no autenticado")
	ErrForbidden         = errors.New("no tienes permisos para realizar esta acción")
	ErrIdeaNotFound      = errors.New("idea no encontrada")
	ErrInvalidPermission = errors.New("permiso inválido")
)

type Actor struct {
	UserID      uuid.UUID
	Email       string
	IsSuperuser bool
	RoleIDs     []uuid.UUID
	Roles       []database.Role
}

func NewActorFromUser(user database.User, roles []database.Role) Actor {
	roleIDs := make([]uuid.UUID, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
	}
	return Actor{
		UserID:      user.ID,
		Email:       user.Email,
		IsSuperuser: user.IsSuperuser == 1,
		RoleIDs:     roleIDs,
		Roles:       roles,
	}
}

func (a Actor) IsValid() bool {
	return a.UserID != uuid.Nil
}

func (a Actor) CanManageUsersAndRoles() bool {
	return a.IsValid() && a.IsSuperuser
}

func (a Actor) CanManageTopics() bool {
	return a.IsValid() && a.IsSuperuser
}

func (a Actor) CanDeleteIdea(idea database.Idea) bool {
	return a.IsValid() && (a.IsSuperuser || idea.UserID == a.UserID)
}

func (a Actor) CanManageIdeaPermissions(idea database.Idea) bool {
	return a.IsValid() && (a.IsSuperuser || idea.UserID == a.UserID)
}

func IsValidPermission(perm string) bool {
	p := strings.ToLower(strings.TrimSpace(perm))
	return p == PermissionView || p == PermissionEdit
}

func NormalizePermission(perm string) (string, bool) {
	p := strings.ToLower(strings.TrimSpace(perm))
	if p == PermissionView || p == PermissionEdit {
		return p, true
	}
	return "", false
}

func CanViewIdea(ctx context.Context, queries *database.Queries, actor Actor, idea database.Idea) (bool, error) {
	if !actor.IsValid() {
		return false, ErrUnauthorized
	}
	if actor.IsSuperuser || idea.UserID == actor.UserID {
		return true, nil
	}
	count, err := queries.HasUserViewPermissionOnIdea(ctx, database.HasUserViewPermissionOnIdeaParams{
		ID:     idea.ID,
		UserID: actor.UserID,
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func CanEditIdea(ctx context.Context, queries *database.Queries, actor Actor, idea database.Idea) (bool, error) {
	if !actor.IsValid() {
		return false, ErrUnauthorized
	}
	if actor.IsSuperuser || idea.UserID == actor.UserID {
		return true, nil
	}
	count, err := queries.HasUserEditPermissionOnIdea(ctx, database.HasUserEditPermissionOnIdeaParams{
		ID:     idea.ID,
		UserID: actor.UserID,
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func ListAccessibleIdeaIDSet(ctx context.Context, queries *database.Queries, actor Actor) (map[uuid.UUID]bool, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}
	var (
		ideas []database.Idea
		err   error
	)
	if actor.IsSuperuser {
		ideas, err = queries.ListIdeas(ctx)
	} else {
		ideas, err = queries.ListAccessibleIdeasByUser(ctx, actor.UserID)
	}
	if err != nil {
		return nil, err
	}
	set := make(map[uuid.UUID]bool, len(ideas))
	for _, idea := range ideas {
		set[idea.ID] = true
	}
	return set, nil
}
