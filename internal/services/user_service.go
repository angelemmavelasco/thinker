package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"thinker/internal/auth"
	"thinker/internal/database"
)

type UserService struct {
	queries *database.Queries
}

func NewUserService(queries *database.Queries) *UserService {
	return &UserService{queries: queries}
}

func (s *UserService) ListUsers(ctx context.Context) ([]database.ListUsersRow, error) {
	return s.queries.ListUsers(ctx)
}

func (s *UserService) CreateUser(ctx context.Context, rawEmail, password, role string) (database.User, error) {
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

	cleanedRole := strings.TrimSpace(role)
	if cleanedRole != "admin" && cleanedRole != "member" {
		cleanedRole = "member"
	}

	return s.queries.CreateUser(ctx, database.CreateUserParams{
		ID:            uuid.New(),
		Email:         normalizedEmail,
		PasswordHash:  hashedPassword,
		Role:          cleanedRole,
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 1,
	})
}
