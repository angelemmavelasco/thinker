package services

import (
	"context"

	"thinker/internal/database"
	"thinker/internal/models"
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

// func (s *IdeaService) CreateIdea(ctx context.Context, input models.Idea) (database.Idea, error) {
// 	var userID 
// }
