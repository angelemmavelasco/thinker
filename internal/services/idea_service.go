package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/google/uuid"
	"thinker/internal/database"
)

const (
	defaultSemanticThreshold = 0.70
	defaultSemanticTopK      = 5
)

type IdeaWithKeywords struct {
	database.Idea
	Keywords []database.Keyword
}

type GraphNode struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Content  string   `json:"content,omitempty"`
	Status   string   `json:"status,omitempty"`
	UserID   string   `json:"userId,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

type GraphLink struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
}

type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

type semanticCandidate struct {
	ideaID uuid.UUID
	score  float64
}

type ideaJob struct {
	idea    database.Idea
	title   string
	content string
}

type IdeaService struct {
	queries        *database.Queries
	llm            *LLMService
	embedding      *EmbeddingService
	keywordService *KeywordService
	jobQueue       chan ideaJob
	inFlight       sync.Map
}

func NewIdeaService(queries *database.Queries, llm *LLMService, embedding *EmbeddingService) *IdeaService {
	svc := &IdeaService{
		queries:        queries,
		llm:            llm,
		embedding:      embedding,
		keywordService: NewKeywordService(queries, embedding),
		jobQueue:       make(chan ideaJob, 100),
	}
	go svc.startWorker()
	return svc
}

func (s *IdeaService) startWorker() {
	for job := range s.jobQueue {
		idKey := job.idea.ID.String()
		log.Printf("[llm] Iniciando análisis para la idea [%s] (%s)...", job.idea.ID, job.title)
		result, savedKeywords, links, err := s.ProcessIdeaAnalysis(context.Background(), job.idea.ID, job.title, job.content)
		s.inFlight.Delete(idKey)
		if err != nil {
			log.Printf("[llm] Error procesando idea [%s]: %v", job.idea.ID, err)
			continue
		}
		log.Printf("[llm] Análisis y persistencia completados para [%s]:", job.idea.ID)
		log.Printf("  -> Nota reestructurada: %s", result.RestructuredNote)
		log.Printf("  -> Keywords guardados (%d): %v", len(savedKeywords), result.KeyEntities)
		log.Printf("  -> Conexiones creadas/actualizadas (%d)", len(links))
	}
}

func (s *IdeaService) EnqueueIdeaAnalysis(idea database.Idea, title, content string) {
	idKey := idea.ID.String()
	if _, loaded := s.inFlight.LoadOrStore(idKey, true); loaded {
		return
	}
	select {
	case s.jobQueue <- ideaJob{idea: idea, title: title, content: content}:
	default:
		go func() {
			s.jobQueue <- ideaJob{idea: idea, title: title, content: content}
		}()
	}
}

func (s *IdeaService) ListIdeas(ctx context.Context) ([]IdeaWithKeywords, error) {
	ideas, err := s.queries.ListIdeas(ctx)
	if err != nil {
		return nil, err
	}

	splitOccurred := false
	for _, idea := range ideas {
		if (idea.Status == "borrador" || idea.Status == "draft") && len([]rune(idea.Content)) > DefaultChunkMaxChars {
			if _, err := s.CreateIdeasFromInput(ctx, idea.UserID.String(), idea.Title, idea.Content); err == nil {
				_ = s.queries.DeleteIdea(ctx, idea.ID)
				splitOccurred = true
			}
		}
	}

	if splitOccurred {
		ideas, err = s.queries.ListIdeas(ctx)
		if err != nil {
			return nil, err
		}
	}

	items := make([]IdeaWithKeywords, 0, len(ideas))
	for _, idea := range ideas {
		if idea.Status == "borrador" || idea.Status == "draft" {
			s.EnqueueIdeaAnalysis(idea, idea.Title, idea.Content)
		}
		kws, err := s.keywordService.ListKeywordsByIdeaID(ctx, idea.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, IdeaWithKeywords{
			Idea:     idea,
			Keywords: kws,
		})
	}

	return items, nil
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
		Status:  "borrador",
	})
}

func (s *IdeaService) CreateIdeasFromInput(ctx context.Context, userIDStr, title, content string) ([]database.Idea, error) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, err
	}

	chunks := ChunkText(content, DefaultChunkMaxChars)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("el contenido está vacío")
	}

	created := make([]database.Idea, 0, len(chunks))
	for i, chunk := range chunks {
		chunkTitle := title
		if len(chunks) > 1 {
			chunkTitle = fmt.Sprintf("%s (%d/%d)", title, i+1, len(chunks))
		}

		idea, err := s.queries.CreateIdea(ctx, database.CreateIdeaParams{
			ID:      uuid.New(),
			UserID:  userID,
			Title:   chunkTitle,
			Content: chunk,
			Status:  "borrador",
		})
		if err != nil {
			return nil, err
		}

		_, _ = s.queries.CreateIdeaChunk(ctx, database.CreateIdeaChunkParams{
			ID:         uuid.New(),
			IdeaID:     idea.ID,
			ChunkIndex: int32(i),
			Content:    chunk,
			Embedding:  "",
		})

		created = append(created, idea)
	}

	for i := 1; i < len(created); i++ {
		prevID := created[i-1].ID
		currID := created[i].ID
		_, _ = s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: prevID,
			TargetIdeaID: currID,
			RelationType: "semantic",
			Weight:       0.90,
		})
		_, _ = s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: currID,
			TargetIdeaID: prevID,
			RelationType: "semantic",
			Weight:       0.90,
		})
	}

	for _, idea := range created {
		s.EnqueueIdeaAnalysis(idea, idea.Title, idea.Content)
	}

	return created, nil
}

func (s *IdeaService) AnalyzeIdea(ctx context.Context, title, content string) (*IdeaAnalysisResult, error) {
	return s.llm.AnalyzeIdea(ctx, title, content)
}

func (s *IdeaService) LinkOverlappingIdeas(ctx context.Context, ideaID uuid.UUID) ([]database.IdeaLink, error) {
	overlaps, err := s.queries.FindOverlappingIdeas(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	var links []database.IdeaLink
	for _, overlap := range overlaps {
		weight := float64(overlap.SharedCount)

		forwardLink, err := s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: ideaID,
			TargetIdeaID: overlap.IdeaID,
			RelationType: "concept_overlap",
			Weight:       weight,
		})
		if err != nil {
			return nil, err
		}

		backwardLink, err := s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: overlap.IdeaID,
			TargetIdeaID: ideaID,
			RelationType: "concept_overlap",
			Weight:       weight,
		})
		if err != nil {
			return nil, err
		}

		links = append(links, forwardLink, backwardLink)
	}

	return links, nil
}

func (s *IdeaService) LinkSemanticIdeas(ctx context.Context, ideaID uuid.UUID, ideaVec []float64, threshold float64, topK int) ([]database.IdeaLink, error) {
	if len(ideaVec) == 0 {
		return nil, nil
	}

	others, err := s.queries.ListIdeasWithEmbeddings(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	var candidates []semanticCandidate
	for _, other := range others {
		otherVec, err := DeserializeVector(other.Embedding)
		if err != nil || len(otherVec) == 0 {
			continue
		}

		score := CosineSimilarity(ideaVec, otherVec)
		if score >= threshold {
			candidates = append(candidates, semanticCandidate{
				ideaID: other.ID,
				score:  score,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}

	var links []database.IdeaLink
	for _, cand := range candidates {
		forwardLink, err := s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: ideaID,
			TargetIdeaID: cand.ideaID,
			RelationType: "semantic",
			Weight:       cand.score,
		})
		if err != nil {
			return nil, err
		}

		backwardLink, err := s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: cand.ideaID,
			TargetIdeaID: ideaID,
			RelationType: "semantic",
			Weight:       cand.score,
		})
		if err != nil {
			return nil, err
		}

		links = append(links, forwardLink, backwardLink)
	}

	return links, nil
}

func (s *IdeaService) ProcessIdeaAnalysis(ctx context.Context, ideaID uuid.UUID, title, content string) (*IdeaAnalysisResult, []database.Keyword, []database.IdeaLink, error) {
	result, err := s.llm.AnalyzeIdea(ctx, title, content)
	if err != nil {
		return nil, nil, nil, err
	}

	var ideaVec []float64
	var serializedEmbedding string
	if s.embedding != nil {
		textForEmbedding := title + "\n\n" + result.RestructuredNote
		vec, embErr := s.embedding.GenerateEmbedding(ctx, textForEmbedding)
		if embErr != nil {
			log.Printf("[embedding] Error generando embedding para idea [%s]: %v", ideaID, embErr)
		} else {
			ideaVec = vec
			serializedEmbedding, _ = SerializeVector(vec)
		}
	}

	if _, err := s.queries.UpdateIdeaAnalysis(ctx, database.UpdateIdeaAnalysisParams{
		ID:                  ideaID,
		RestructuredContent: result.RestructuredNote,
		Embedding:           serializedEmbedding,
		Status:              "procesada",
	}); err != nil {
		return result, nil, nil, err
	}

	keywords, err := s.keywordService.SaveKeywordsForIdea(ctx, ideaID, result.KeyEntities)
	if err != nil {
		return result, nil, nil, err
	}

	overlapLinks, err := s.LinkOverlappingIdeas(ctx, ideaID)
	if err != nil {
		return result, keywords, nil, err
	}

	semanticLinks, err := s.LinkSemanticIdeas(ctx, ideaID, ideaVec, defaultSemanticThreshold, defaultSemanticTopK)
	if err != nil {
		return result, keywords, overlapLinks, err
	}

	allLinks := append(overlapLinks, semanticLinks...)
	return result, keywords, allLinks, nil
}

func (s *IdeaService) ProcessPendingIdeas(ctx context.Context) {
	ideas, err := s.queries.ListIdeas(ctx)
	if err != nil {
		return
	}
	for _, idea := range ideas {
		if idea.Status == "borrador" || idea.Status == "draft" {
			if len([]rune(idea.Content)) > DefaultChunkMaxChars {
				if _, err := s.CreateIdeasFromInput(ctx, idea.UserID.String(), idea.Title, idea.Content); err == nil {
					_ = s.queries.DeleteIdea(ctx, idea.ID)
					continue
				}
			}
			s.EnqueueIdeaAnalysis(idea, idea.Title, idea.Content)
		}
	}
}

func (s *IdeaService) GetNetworkGraph(ctx context.Context) (*GraphData, error) {
	ideasWithKws, err := s.ListIdeas(ctx)
	if err != nil {
		return nil, err
	}

	rawLinks, err := s.queries.ListAllIdeaLinks(ctx)
	if err != nil {
		return nil, err
	}

	nodes := make([]GraphNode, 0, len(ideasWithKws))
	links := make([]GraphLink, 0, len(rawLinks))
	keywordNodes := make(map[string]GraphNode)

	for _, item := range ideasWithKws {
		ideaID := item.ID.String()
		displayContent := item.Content
		if item.RestructuredContent != "" {
			displayContent = item.RestructuredContent
		}

		kwNames := make([]string, 0, len(item.Keywords))
		for _, kw := range item.Keywords {
			kwNames = append(kwNames, kw.Name)
			kwNodeID := "kw:" + kw.ID.String()
			if _, exists := keywordNodes[kwNodeID]; !exists {
				keywordNodes[kwNodeID] = GraphNode{
					ID:    kwNodeID,
					Label: kw.Name,
					Type:  "keyword",
				}
			}
			links = append(links, GraphLink{
				Source: ideaID,
				Target: kwNodeID,
				Type:   "has_keyword",
				Weight: 1.0,
			})
		}

		nodes = append(nodes, GraphNode{
			ID:       ideaID,
			Label:    item.Title,
			Type:     "idea",
			Content:  displayContent,
			Status:   item.Status,
			UserID:   item.UserID.String(),
			Keywords: kwNames,
		})
	}

	for _, kwNode := range keywordNodes {
		nodes = append(nodes, kwNode)
	}

	seenPairs := make(map[string]struct{})
	for _, l := range rawLinks {
		src := l.SourceIdeaID.String()
		dst := l.TargetIdeaID.String()
		a, b := src, dst
		if a > b {
			a, b = b, a
		}
		pairKey := a + "|" + b + "|" + l.RelationType
		if _, exists := seenPairs[pairKey]; exists {
			continue
		}
		seenPairs[pairKey] = struct{}{}

		links = append(links, GraphLink{
			Source: src,
			Target: dst,
			Type:   l.RelationType,
			Weight: l.Weight,
		})
	}

	return &GraphData{
		Nodes: nodes,
		Links: links,
	}, nil
}
