package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

func (s *UserService) CreateUser(ctx context.Context, email, password, role string) (database.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return database.User{}, err
	}

	cleanedRole := strings.TrimSpace(role)
	if cleanedRole == "" {
		cleanedRole = "member"
	}

	return s.queries.CreateUser(ctx, database.CreateUserParams{
		ID:           uuid.New(),
		Email:        strings.TrimSpace(email),
		PasswordHash: string(hashedPassword),
		Role:         cleanedRole,
	})
}
