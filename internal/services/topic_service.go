package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"thinker/internal/database"
)

type TopicService struct {
	queries   *database.Queries
	embedding *EmbeddingService
	llm       *LLMService
}

func NewTopicService(queries *database.Queries, embedding *EmbeddingService, llm *LLMService) *TopicService {
	return &TopicService{
		queries:   queries,
		embedding: embedding,
		llm:       llm,
	}
}

func (s *TopicService) GetSetting(ctx context.Context, key string, fallback string) string {
	val, err := s.queries.GetKnowledgeSetting(ctx, key)
	if err != nil {
		return fallback
	}
	return val.Value
}

func (s *TopicService) GetBoolSetting(ctx context.Context, key string, fallback bool) bool {
	val := s.GetSetting(ctx, key, "")
	if val == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "t":
		return true
	case "false", "0", "no", "f":
		return false
	default:
		return fallback
	}
}

func (s *TopicService) GetFloatSetting(ctx context.Context, key string, fallback float64) float64 {
	val := s.GetSetting(ctx, key, "")
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *TopicService) GetIntSetting(ctx context.Context, key string, fallback int) int {
	val := s.GetSetting(ctx, key, "")
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *TopicService) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.queries.UpsertKnowledgeSetting(ctx, database.UpsertKnowledgeSettingParams{
		Key:   key,
		Value: value,
	})
	return err
}

func (s *TopicService) ListMOCs(ctx context.Context) ([]database.KnowledgeMoc, error) {
	return s.queries.ListKnowledgeMOCs(ctx)
}

func (s *TopicService) GetMOCByID(ctx context.Context, id uuid.UUID) (database.KnowledgeMoc, error) {
	return s.queries.GetKnowledgeMOCByID(ctx, id)
}

func (s *TopicService) GetMOCByCode(ctx context.Context, code string) (database.KnowledgeMoc, error) {
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	return s.queries.GetKnowledgeMOCByCode(ctx, cleanCode)
}

func (s *TopicService) CreateMOC(ctx context.Context, code, title, description string, isLocked bool) (database.KnowledgeMoc, error) {
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	if !strings.HasPrefix(cleanCode, "MOC_") {
		cleanCode = "MOC_" + cleanCode
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = cleanCode
	}
	cleanDesc := strings.TrimSpace(description)

	var serializedEmbedding string
	if s.embedding != nil {
		embText := cleanTitle + "\n\n" + cleanDesc
		if vec, err := s.embedding.GenerateEmbedding(ctx, embText); err == nil && len(vec) > 0 {
			serializedEmbedding, _ = SerializeVector(vec)
		}
	}

	lockedVal := int32(0)
	if isLocked {
		lockedVal = 1
	}

	total, _ := s.queries.CountKnowledgeMOCs(ctx)

	return s.queries.CreateKnowledgeMOC(ctx, database.CreateKnowledgeMOCParams{
		ID:           uuid.New(),
		Code:         cleanCode,
		Title:        cleanTitle,
		Description:  cleanDesc,
		Embedding:    serializedEmbedding,
		DisplayOrder: int32(total + 1),
		IsLocked:     lockedVal,
	})
}

func (s *TopicService) UpdateMOC(ctx context.Context, id uuid.UUID, title, description string, displayOrder int32, isLocked bool) (database.KnowledgeMoc, error) {
	current, err := s.queries.GetKnowledgeMOCByID(ctx, id)
	if err != nil {
		return database.KnowledgeMoc{}, err
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = current.Title
	}
	cleanDesc := strings.TrimSpace(description)

	serializedEmbedding := current.Embedding
	if s.embedding != nil && (cleanTitle != current.Title || cleanDesc != current.Description || current.Embedding == "") {
		embText := cleanTitle + "\n\n" + cleanDesc
		if vec, err := s.embedding.GenerateEmbedding(ctx, embText); err == nil && len(vec) > 0 {
			serializedEmbedding, _ = SerializeVector(vec)
		}
	}

	lockedVal := int32(0)
	if isLocked {
		lockedVal = 1
	}

	return s.queries.UpdateKnowledgeMOC(ctx, database.UpdateKnowledgeMOCParams{
		ID:           id,
		Title:        cleanTitle,
		Description:  cleanDesc,
		Embedding:    serializedEmbedding,
		DisplayOrder: displayOrder,
		IsLocked:     lockedVal,
	})
}

func (s *TopicService) DeleteMOC(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteKnowledgeMOC(ctx, id)
}

func (s *TopicService) ListHubsByMOC(ctx context.Context, mocID uuid.UUID) ([]database.KnowledgeHub, error) {
	return s.queries.ListKnowledgeHubsByMOCID(ctx, mocID)
}

func (s *TopicService) ListAllHubs(ctx context.Context) ([]database.KnowledgeHub, error) {
	return s.queries.ListAllKnowledgeHubs(ctx)
}

func (s *TopicService) GetHubByID(ctx context.Context, id uuid.UUID) (database.KnowledgeHub, error) {
	return s.queries.GetKnowledgeHubByID(ctx, id)
}

func (s *TopicService) CreateHub(ctx context.Context, mocID uuid.UUID, title, summary string, aliases []string, isLocked bool) (database.KnowledgeHub, error) {
	cleanTitle := strings.TrimSpace(title)
	cleanSummary := strings.TrimSpace(summary)

	cleanAliases := sanitizeAliases(aliases)
	rawAliases, _ := json.Marshal(cleanAliases)

	var serializedEmbedding string
	if s.embedding != nil {
		embText := cleanTitle + "\n\n" + cleanSummary
		if vec, err := s.embedding.GenerateEmbedding(ctx, embText); err == nil && len(vec) > 0 {
			serializedEmbedding, _ = SerializeVector(vec)
		}
	}

	lockedVal := int32(0)
	if isLocked {
		lockedVal = 1
	}

	count, _ := s.queries.CountKnowledgeHubsByMOCID(ctx, mocID)

	return s.queries.CreateKnowledgeHub(ctx, database.CreateKnowledgeHubParams{
		ID:            uuid.New(),
		MocID:         mocID,
		Title:         cleanTitle,
		Summary:       cleanSummary,
		SearchAliases: string(rawAliases),
		Embedding:     serializedEmbedding,
		DisplayOrder:  int32(count + 1),
		IsLocked:      lockedVal,
	})
}

func (s *TopicService) UpdateHub(ctx context.Context, id uuid.UUID, mocID uuid.UUID, title, summary string, aliases []string, displayOrder int32, isLocked bool) (database.KnowledgeHub, error) {
	current, err := s.queries.GetKnowledgeHubByID(ctx, id)
	if err != nil {
		return database.KnowledgeHub{}, err
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = current.Title
	}
	cleanSummary := strings.TrimSpace(summary)

	cleanAliases := sanitizeAliases(aliases)
	rawAliases, _ := json.Marshal(cleanAliases)

	serializedEmbedding := current.Embedding
	if s.embedding != nil && (cleanTitle != current.Title || cleanSummary != current.Summary || current.Embedding == "") {
		embText := cleanTitle + "\n\n" + cleanSummary
		if vec, err := s.embedding.GenerateEmbedding(ctx, embText); err == nil && len(vec) > 0 {
			serializedEmbedding, _ = SerializeVector(vec)
		}
	}

	lockedVal := int32(0)
	if isLocked {
		lockedVal = 1
	}

	return s.queries.UpdateKnowledgeHub(ctx, database.UpdateKnowledgeHubParams{
		ID:            id,
		MocID:         mocID,
		Title:         cleanTitle,
		Summary:       cleanSummary,
		SearchAliases: string(rawAliases),
		Embedding:     serializedEmbedding,
		DisplayOrder:  displayOrder,
		IsLocked:      lockedVal,
	})
}

func (s *TopicService) DeleteHub(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteKnowledgeHub(ctx, id)
}

func (s *TopicService) LinkIdeaToHub(ctx context.Context, hubID, ideaID uuid.UUID, relType string, isManualOverride bool) error {
	cleanRel := strings.TrimSpace(relType)
	if cleanRel == "" {
		cleanRel = "GOVERNS"
	}

	overrideVal := int32(0)
	if isManualOverride {
		overrideVal = 1
	}

	_, err := s.queries.CreateHubIdeaEdge(ctx, database.CreateHubIdeaEdgeParams{
		HubID:            hubID,
		IdeaID:           ideaID,
		RelationshipType: cleanRel,
		Weight:           1.0,
		IsManualOverride: overrideVal,
	})
	return err
}

func (s *TopicService) UnlinkIdeaFromHub(ctx context.Context, hubID, ideaID uuid.UUID) error {
	return s.queries.DeleteHubIdeaEdge(ctx, database.DeleteHubIdeaEdgeParams{
		HubID:  hubID,
		IdeaID: ideaID,
	})
}

func (s *TopicService) ListIdeasByHub(ctx context.Context, hubID uuid.UUID) ([]database.ListIdeasByHubIDRow, error) {
	return s.queries.ListIdeasByHubID(ctx, hubID)
}

func (s *TopicService) ListHubsByIdea(ctx context.Context, ideaID uuid.UUID) ([]database.ListHubsByIdeaIDRow, error) {
	return s.queries.ListHubsByIdeaID(ctx, ideaID)
}

func (s *TopicService) MoveIdeaToHub(ctx context.Context, ideaID, targetMocID, targetHubID uuid.UUID) error {
	if err := s.queries.DeleteHubIdeaEdgesByIdeaID(ctx, ideaID); err != nil {
		return err
	}
	return s.LinkIdeaToHub(ctx, targetHubID, ideaID, "GOVERNS", true)
}

func (s *TopicService) ResolveAndLinkIdea(ctx context.Context, ideaID uuid.UUID, analysis *IdeaAnalysisResult) (*database.KnowledgeMoc, *database.KnowledgeHub, error) {
	if analysis == nil {
		return nil, nil, nil
	}

	rawMocCode := strings.ToUpper(strings.TrimSpace(analysis.TargetMOCCode))
	if rawMocCode == "" {
		return nil, nil, nil
	}
	if !strings.HasPrefix(rawMocCode, "MOC_") {
		rawMocCode = "MOC_" + rawMocCode
	}

	// 1. Resolve MOC
	var targetMOC database.KnowledgeMoc
	moc, err := s.queries.GetKnowledgeMOCByCode(ctx, rawMocCode)
	if err == nil {
		targetMOC = moc
	} else if errors.Is(err, sql.ErrNoRows) {
		allowAuto := s.GetBoolSetting(ctx, "allow_auto_moc_creation", true)
		maxMOCs := s.GetIntSetting(ctx, "max_auto_mocs", 15)
		count, _ := s.queries.CountKnowledgeMOCs(ctx)

		if allowAuto && int(count) < maxMOCs {
			mocTitle := strings.TrimSpace(analysis.TargetMOCTitle)
			if mocTitle == "" {
				mocTitle = formatMocCodeToTitle(rawMocCode)
			}
			mocDesc := strings.TrimSpace(analysis.TargetMOCDesc)
			if mocDesc == "" {
				mocDesc = "Dominio temático para " + mocTitle
			}
			newMOC, createErr := s.CreateMOC(ctx, rawMocCode, mocTitle, mocDesc, false)
			if createErr != nil {
				return nil, nil, createErr
			}
			targetMOC = newMOC
		} else {
			allMOCs, listErr := s.queries.ListKnowledgeMOCs(ctx)
			if listErr != nil || len(allMOCs) == 0 {
				return nil, nil, nil
			}
			targetMOC = allMOCs[0]
		}
	} else {
		return nil, nil, err
	}

	// 2. Resolve Hub
	propHubTitle := strings.TrimSpace(analysis.ProposedHubTitle)
	if propHubTitle == "" {
		return &targetMOC, nil, nil
	}
	propHubSummary := strings.TrimSpace(analysis.HubSummary)
	if propHubSummary == "" {
		propHubSummary = analysis.RestructuredNote
	}

	var propHubVec []float64
	if s.embedding != nil {
		textForHub := propHubTitle + "\n\n" + propHubSummary
		if vec, err := s.embedding.GenerateEmbedding(ctx, textForHub); err == nil && len(vec) > 0 {
			propHubVec = vec
		}
	}

	hubs, err := s.queries.ListKnowledgeHubsByMOCID(ctx, targetMOC.ID)
	if err != nil {
		return &targetMOC, nil, err
	}

	threshold := s.GetFloatSetting(ctx, "hub_dedup_similarity_threshold", 0.82)
	var bestMatch *database.KnowledgeHub
	var bestScore float64 = -1.0

	for _, h := range hubs {
		if strings.EqualFold(strings.TrimSpace(h.Title), propHubTitle) {
			match := h
			bestMatch = &match
			bestScore = 1.0
			break
		}
		if len(propHubVec) > 0 && h.Embedding != "" {
			if existingVec, err := DeserializeVector(h.Embedding); err == nil && len(existingVec) > 0 {
				score := CosineSimilarity(propHubVec, existingVec)
				if score > bestScore {
					scoreCopy := score
					match := h
					bestMatch = &match
					bestScore = scoreCopy
				}
			}
		}
	}

	var targetHub database.KnowledgeHub
	if bestMatch != nil && bestScore >= threshold {
		// Deduplication: Reuse existing hub and append new aliases
		targetHub = *bestMatch
		existingAliases := parseAliases(targetHub.SearchAliases)
		mergedAliases := mergeAliases(existingAliases, analysis.SearchAliases)
		rawMerged, _ := json.Marshal(mergedAliases)

		_, _ = s.queries.UpdateKnowledgeHub(ctx, database.UpdateKnowledgeHubParams{
			ID:            targetHub.ID,
			MocID:         targetHub.MocID,
			Title:         targetHub.Title,
			Summary:       targetHub.Summary,
			SearchAliases: string(rawMerged),
			Embedding:     targetHub.Embedding,
			DisplayOrder:  targetHub.DisplayOrder,
			IsLocked:      targetHub.IsLocked,
		})
	} else {
		// Create new hub under targetMOC
		newHub, err := s.CreateHub(ctx, targetMOC.ID, propHubTitle, propHubSummary, analysis.SearchAliases, false)
		if err != nil {
			return &targetMOC, nil, err
		}
		targetHub = newHub
	}

	// 3. Link idea to Hub in hub_idea_edges
	if err := s.LinkIdeaToHub(ctx, targetHub.ID, ideaID, "GOVERNS", false); err != nil {
		log.Printf("[topics] Error vinculando idea [%s] a hub [%s]: %v", ideaID, targetHub.ID, err)
		return &targetMOC, &targetHub, err
	}

	return &targetMOC, &targetHub, nil
}

func sanitizeAliases(aliases []string) []string {
	seen := make(map[string]bool)
	var clean []string
	for _, a := range aliases {
		norm := strings.ToLower(strings.TrimSpace(a))
		if norm != "" && !seen[norm] {
			seen[norm] = true
			clean = append(clean, norm)
		}
	}
	if clean == nil {
		clean = make([]string, 0)
	}
	return clean
}

func parseAliases(raw string) []string {
	var aliases []string
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &aliases)
	}
	if aliases == nil {
		aliases = make([]string, 0)
	}
	return aliases
}

func mergeAliases(a, b []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, item := range a {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm != "" && !seen[norm] {
			seen[norm] = true
			res = append(res, norm)
		}
	}
	for _, item := range b {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm != "" && !seen[norm] {
			seen[norm] = true
			res = append(res, norm)
		}
	}
	if res == nil {
		res = make([]string, 0)
	}
	return res
}

func formatMocCodeToTitle(code string) string {
	clean := strings.TrimPrefix(code, "MOC_")
	parts := strings.Split(clean, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, " ")
}
