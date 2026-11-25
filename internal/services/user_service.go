package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"thinker/internal/auth"
	"thinker/internal/database"
)

var (
	ErrCannotDemoteLastSuperuser = errors.New("debe existir al menos un superusuario activo")
	ErrCannotSelfDemote          = errors.New("no puedes retirar tus propios privilegios de superusuario")
	ErrUserNotFound              = errors.New("usuario no encontrado")
)

type UserRoleBadge struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type UserWithRoles struct {
	ID            uuid.UUID       `json:"id"`
	Email         string          `json:"email"`
	Role          string          `json:"role"`
	IsSuperuser   bool            `json:"isSuperuser"`
	AuthProvider  string          `json:"authProvider"`
	EmailVerified bool            `json:"emailVerified"`
	CreatedAt     time.Time       `json:"createdAt"`
	Roles         []UserRoleBadge `json:"roles"`
	RoleIDMap     map[string]bool `json:"-"`
}

type UserService struct {
	queries *database.Queries
}

func NewUserService(queries *database.Queries) *UserService {
	return &UserService{queries: queries}
}

func (s *UserService) ListUsers(ctx context.Context, actor Actor) ([]UserWithRoles, error) {
	if !actor.CanManageUsersAndRoles() {
		return nil, ErrForbidden
	}

	rawUsers, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	allUserRoles, err := s.queries.ListAllUserRoles(ctx)
	if err != nil {
		return nil, err
	}

	rolesByUser := make(map[uuid.UUID][]UserRoleBadge)
	roleMapByUser := make(map[uuid.UUID]map[string]bool)
	for _, row := range allUserRoles {
		rolesByUser[row.UserID] = append(rolesByUser[row.UserID], UserRoleBadge{
			ID:   row.RoleID,
			Name: row.RoleName,
			Slug: row.RoleSlug,
		})
		if roleMapByUser[row.UserID] == nil {
			roleMapByUser[row.UserID] = make(map[string]bool)
		}
		roleMapByUser[row.UserID][row.RoleID.String()] = true
	}

	result := make([]UserWithRoles, 0, len(rawUsers))
	for _, u := range rawUsers {
		rMap := roleMapByUser[u.ID]
		if rMap == nil {
			rMap = make(map[string]bool)
		}
		result = append(result, UserWithRoles{
			ID:            u.ID,
			Email:         u.Email,
			Role:          u.Role,
			IsSuperuser:   u.IsSuperuser == 1,
			AuthProvider:  u.AuthProvider,
			EmailVerified: u.EmailVerified == 1,
			CreatedAt:     u.CreatedAt,
			Roles:         rolesByUser[u.ID],
			RoleIDMap:     rMap,
		})
	}

	return result, nil
}

func (s *UserService) CreateUser(ctx context.Context, actor Actor, rawEmail, password string, isSuperuser bool, roleIDs []uuid.UUID) (database.User, error) {
	if !actor.CanManageUsersAndRoles() {
		return database.User{}, ErrForbidden
	}

	normalizedEmail, err := auth.NormalizeEmail(rawEmail)
	if err != nil {
		return database.User{}, err
	}
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return database.User{}, err
	}

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return database.User{}, err
	}

	cleanedRole := "member"
	superVal := int32(0)
	if isSuperuser {
		cleanedRole = "admin"
		superVal = 1
	}

	uniqueRoleIDs := dedupeUUIDs(roleIDs)

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return database.User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	created, err := qtx.CreateUserWithSuperuser(ctx, database.CreateUserWithSuperuserParams{
		ID:            uuid.New(),
		Email:         normalizedEmail,
		PasswordHash:  hashedPassword,
		Role:          cleanedRole,
		IsSuperuser:   superVal,
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 1,
	})
	if err != nil {
		return database.User{}, err
	}

	for _, roleID := range uniqueRoleIDs {
		if _, err := qtx.GetRoleByID(ctx, roleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return database.User{}, ErrRoleNotFound
			}
			return database.User{}, err
		}
		if err := qtx.AssignRoleToUser(ctx, database.AssignRoleToUserParams{
			UserID:     created.ID,
			RoleID:     roleID,
			AssignedBy: uuid.NullUUID{UUID: actor.UserID, Valid: true},
		}); err != nil {
			return database.User{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return database.User{}, err
	}

	return created, nil
}

func (s *UserService) UpdateUserAccess(ctx context.Context, actor Actor, targetUserID uuid.UUID, isSuperuser bool, roleIDs []uuid.UUID) (database.User, error) {
	if !actor.CanManageUsersAndRoles() {
		return database.User{}, ErrForbidden
	}

	targetUser, err := s.queries.GetUserByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return database.User{}, ErrUserNotFound
		}
		return database.User{}, err
	}

	if targetUser.ID == actor.UserID && !isSuperuser {
		return database.User{}, ErrCannotSelfDemote
	}

	wasSuperuser := targetUser.IsSuperuser == 1
	if wasSuperuser && !isSuperuser {
		count, err := s.queries.CountSuperusers(ctx)
		if err != nil {
			return database.User{}, err
		}
		if count <= 1 {
			return database.User{}, ErrCannotDemoteLastSuperuser
		}
	}

	newRole := "member"
	superVal := int32(0)
	if isSuperuser {
		newRole = "admin"
		superVal = 1
	}

	uniqueRoleIDs := dedupeUUIDs(roleIDs)

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return database.User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	updated, err := qtx.UpdateUserSuperuserStatus(ctx, database.UpdateUserSuperuserStatusParams{
		ID:          targetUserID,
		IsSuperuser: superVal,
		Role:        newRole,
	})
	if err != nil {
		return database.User{}, err
	}

	if err := qtx.DeleteUserRolesByUserID(ctx, targetUserID); err != nil {
		return database.User{}, err
	}

	for _, roleID := range uniqueRoleIDs {
		if _, err := qtx.GetRoleByID(ctx, roleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return database.User{}, ErrRoleNotFound
			}
			return database.User{}, err
		}
		if err := qtx.AssignRoleToUser(ctx, database.AssignRoleToUserParams{
			UserID:     targetUserID,
			RoleID:     roleID,
			AssignedBy: uuid.NullUUID{UUID: actor.UserID, Valid: true},
		}); err != nil {
			return database.User{}, err
		}
	}

	if wasSuperuser && !isSuperuser {
		if _, err := qtx.IncrementUserTokenVersion(ctx, targetUserID); err != nil {
			return database.User{}, err
		}
		if err := qtx.RevokeAllUserRefreshTokens(ctx, database.RevokeAllUserRefreshTokensParams{
			UserID:    targetUserID,
			RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		}); err != nil {
			return database.User{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return database.User{}, err
	}

	return updated, nil
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	res := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		res = append(res, id)
	}
	return res
}
