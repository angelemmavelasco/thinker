package services

import (
	"context"

	"github.com/google/uuid"
	"thinker/internal/database"
)

type IdeaService struct {
	queries        *database.Queries
	llm            *LLMService
	keywordService *KeywordService
}

func NewIdeaService(queries *database.Queries, llm *LLMService) *IdeaService {
	return &IdeaService{
		queries:        queries,
		llm:            llm,
		keywordService: NewKeywordService(queries),
	}
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

func (s *IdeaService) AnalyzeIdea(ctx context.Context, title, content string) (*IdeaAnalysisResult, error) {
	return s.llm.AnalyzeIdea(ctx, title, content)
}

func (s *IdeaService) ProcessIdeaAnalysis(ctx context.Context, ideaID uuid.UUID, title, content string) (*IdeaAnalysisResult, []database.Keyword, error) {
	result, err := s.llm.AnalyzeIdea(ctx, title, content)
	if err != nil {
		return nil, nil, err
	}

	if _, err := s.queries.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaID,
		RestructuredContent: result.RestructuredNote,
		Status:              "processed",
	}); err != nil {
		return result, nil, err
	}

	keywords, err := s.keywordService.SaveKeywordsForIdea(ctx, ideaID, result.KeyEntities)
	if err != nil {
		return result, nil, err
	}

	return result, keywords, nil
}
