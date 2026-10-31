package services

import (
	"context"

	"github.com/google/uuid"
	"thinker/internal/database"
)

type IdeaService struct {
	queries *database.Queries
}

func NewIdeaService(queries *database.Queries) *IdeaService {
	return &IdeaService{queries: queries}
}

func (s *IdeaService) ListIdeas(ctx context.Context) ([]database.Idea, error) {
	return s.queries.ListIdeas(ctx)
}

func (s *IdeaService) CreateIdea(ctx context.Context, userIDStr, title, content string) (database.Idea, error) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return database.Idea{}, err
	}

	return s.queries.CreateIdea(ctx, database.CreateIdeaParams{
		ID:      uuid.New(),
		UserID:  userID,
		Title:   title,
		Content: content,
		Status:  "draft",
	})
}
