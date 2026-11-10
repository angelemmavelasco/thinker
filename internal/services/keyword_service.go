package services

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"
	"thinker/internal/database"
)

type KeywordService struct {
	queries   *database.Queries
	embedding *EmbeddingService
}

func NewKeywordService(queries *database.Queries, embedding *EmbeddingService) *KeywordService {
	return &KeywordService{
		queries:   queries,
		embedding: embedding,
	}
}

func (s *KeywordService) SaveKeywordsForIdea(ctx context.Context, ideaID uuid.UUID, rawEntities []string) ([]database.Keyword, error) {
	seen := make(map[string]struct{})
	var saved []database.Keyword
	rateLimited := false

	for _, raw := range rawEntities {
		cleaned := strings.TrimSpace(raw)
		if cleaned == "" {
			continue
		}

		normalizedKey := strings.ToLower(cleaned)
		if _, exists := seen[normalizedKey]; exists {
			continue
		}
		seen[normalizedKey] = struct{}{}

		kw, err := s.queries.UpsertKeyword(ctx, database.UpsertKeywordParams{
			ID:   uuid.New(),
			Name: cleaned,
		})
		if err != nil {
			return nil, err
		}

		if !rateLimited && s.embedding != nil && strings.TrimSpace(kw.Embedding) == "" {
			vec, embErr := s.embedding.GenerateEmbedding(ctx, kw.Name)
			if embErr != nil {
				log.Printf("[embedding] Error generando embedding para keyword %q: %v", kw.Name, embErr)
				errStr := embErr.Error()
				if strings.Contains(errStr, "1002") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429") {
					rateLimited = true
				}
			} else if serialized, serErr := SerializeVector(vec); serErr == nil && serialized != "" {
				if updatedKw, upErr := s.queries.UpdateKeywordEmbedding(ctx, database.UpdateKeywordEmbeddingParams{
					ID:        kw.ID,
					Embedding: serialized,
				}); upErr == nil {
					kw = updatedKw
				}
			}
		}

		if err := s.queries.LinkIdeaKeyword(ctx, database.LinkIdeaKeywordParams{
			IdeaID:    ideaID,
			KeywordID: kw.ID,
		}); err != nil {
			return nil, err
		}

		saved = append(saved, kw)
	}

	return saved, nil
}

func (s *KeywordService) ListKeywordsByIdeaID(ctx context.Context, ideaID uuid.UUID) ([]database.Keyword, error) {
	return s.queries.ListKeywordsByIdeaID(ctx, ideaID)
}
