package services

import (
	"context"
	"database/sql"
	"errors"
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
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Type            string   `json:"type"`
	Content         string   `json:"content,omitempty"`
	OriginalContent string   `json:"originalContent,omitempty"`
	Status          string   `json:"status,omitempty"`
	UserID          string   `json:"userId,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
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
		if _, err := s.queries.GetIdeaByID(context.Background(), job.idea.ID); err != nil {
			s.inFlight.Delete(idKey)
			continue
		}
		_, _, _, err := s.ProcessIdeaAnalysis(context.Background(), job.idea.ID, job.title, job.content)
		s.inFlight.Delete(idKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[llm] Error procesando idea [%s]: %v", job.idea.ID, err)
			continue
		}
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

	newID := uuid.New()
	return s.queries.CreateIdea(ctx, database.CreateIdeaParams{
		ID:         newID,
		UserID:     userID,
		GroupID:    newID,
		ChunkIndex: 1,
		ChunkTotal: 1,
		Title:      title,
		Content:    content,
		Status:     "borrador",
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

	groupID := uuid.New()
	totalChunks := int32(len(chunks))
	created := make([]database.Idea, 0, len(chunks))

	for i, chunk := range chunks {
		chunkTitle := title
		if len(chunks) > 1 {
			chunkTitle = fmt.Sprintf("%s (%d/%d)", title, i+1, len(chunks))
		}

		newID := uuid.New()
		if len(chunks) == 1 {
			groupID = newID
		}

		idea, err := s.queries.CreateIdea(ctx, database.CreateIdeaParams{
			ID:         newID,
			UserID:     userID,
			GroupID:    groupID,
			ChunkIndex: int32(i + 1),
			ChunkTotal: totalChunks,
			Title:      chunkTitle,
			Content:    chunk,
			Status:     "borrador",
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
			if idea.ChunkTotal <= 1 && len([]rune(idea.Content)) > DefaultChunkMaxChars+50 {
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
			ID:              ideaID,
			Label:           item.Title,
			Type:            "idea",
			Content:         displayContent,
			OriginalContent: item.Content,
			Status:          item.Status,
			UserID:          item.UserID.String(),
			Keywords:        kwNames,
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

type ConnectedIdeaInfo struct {
	ID                  uuid.UUID
	Title               string
	Preview             string
	Status              string
	RelationDescription string
}

type IdeaDetail struct {
	Idea            database.Idea
	Keywords        []database.Keyword
	Connections     []ConnectedIdeaInfo
	GroupIdeas      []database.ListIdeasByGroupIDRow
	GroupCount      int
	CurrentPosition int
	PrevFragmentID  *uuid.UUID
	NextFragmentID  *uuid.UUID
}

func (s *IdeaService) GetIdeaDetail(ctx context.Context, ideaIDStr string) (*IdeaDetail, error) {
	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, err
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	kws, err := s.keywordService.ListKeywordsByIdeaID(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	rawLinks, err := s.queries.ListDirectConnectedIdeas(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	indexByID := make(map[uuid.UUID]int)
	var connections []ConnectedIdeaInfo

	for _, row := range rawLinks {
		relLabel := "Ideas afines"
		if row.RelationType == "concept_overlap" {
			relLabel = "Temas en común"
		}

		if idx, exists := indexByID[row.ID]; exists {
			if connections[idx].RelationDescription != relLabel {
				connections[idx].RelationDescription = "Afinidad y temas en común"
			}
			continue
		}

		preview := row.Content
		if row.RestructuredContent != "" {
			preview = row.RestructuredContent
		}

		indexByID[row.ID] = len(connections)
		connections = append(connections, ConnectedIdeaInfo{
			ID:                  row.ID,
			Title:               row.Title,
			Preview:             preview,
			Status:              row.Status,
			RelationDescription: relLabel,
		})
	}

	var groupIdeas []database.ListIdeasByGroupIDRow
	var currentPos int
	var prevID *uuid.UUID
	var nextID *uuid.UUID

	if idea.GroupID != uuid.Nil {
		groupIdeas, _ = s.queries.ListIdeasByGroupID(ctx, idea.GroupID)
		for i, gi := range groupIdeas {
			if gi.ID == idea.ID {
				currentPos = i + 1
				if i > 0 {
					pid := groupIdeas[i-1].ID
					prevID = &pid
				}
				if i+1 < len(groupIdeas) {
					nid := groupIdeas[i+1].ID
					nextID = &nid
				}
				break
			}
		}
	}

	groupCount := len(groupIdeas)
	if groupCount == 0 {
		groupCount = 1
		currentPos = 1
	}

	return &IdeaDetail{
		Idea:            idea,
		Keywords:        kws,
		Connections:     connections,
		GroupIdeas:      groupIdeas,
		GroupCount:      groupCount,
		CurrentPosition: currentPos,
		PrevFragmentID:  prevID,
		NextFragmentID:  nextID,
	}, nil
}

func (s *IdeaService) relinkGroupSequence(ctx context.Context, groupID uuid.UUID) {
	if groupID == uuid.Nil {
		return
	}
	groupIdeas, err := s.queries.ListIdeasByGroupID(ctx, groupID)
	if err != nil || len(groupIdeas) < 2 {
		return
	}
	for i := 1; i < len(groupIdeas); i++ {
		prevID := groupIdeas[i-1].ID
		currID := groupIdeas[i].ID
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
}

func (s *IdeaService) UpdateIdea(ctx context.Context, ideaIDStr, title, content string) (*IdeaDetail, error) {
	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, err
	}

	_ = s.queries.DeleteIdeaLinksByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteIdeaKeywordsByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteOrphanKeywords(ctx)

	updated, err := s.queries.UpdateIdeaContentAndReset(ctx, database.UpdateIdeaContentAndResetParams{
		ID:      ideaID,
		Title:   title,
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	s.relinkGroupSequence(ctx, updated.GroupID)
	s.EnqueueIdeaAnalysis(updated, updated.Title, updated.Content)

	return s.GetIdeaDetail(ctx, ideaIDStr)
}

func (s *IdeaService) ReanalyzeIdea(ctx context.Context, ideaIDStr string) (*IdeaDetail, error) {
	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, err
	}

	_ = s.queries.DeleteIdeaLinksByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteIdeaKeywordsByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteOrphanKeywords(ctx)

	resetIdea, err := s.queries.ResetIdeaForReanalysis(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	s.relinkGroupSequence(ctx, resetIdea.GroupID)
	s.EnqueueIdeaAnalysis(resetIdea, resetIdea.Title, resetIdea.Content)

	return s.GetIdeaDetail(ctx, ideaIDStr)
}

func (s *IdeaService) DeleteIdeaFragment(ctx context.Context, ideaIDStr string) error {
	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return err
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		return err
	}

	if err := s.queries.DeleteIdea(ctx, ideaID); err != nil {
		return err
	}

	_ = s.queries.DeleteOrphanKeywords(ctx)
	s.relinkGroupSequence(ctx, idea.GroupID)
	return nil
}

func (s *IdeaService) DeleteIdeaGroup(ctx context.Context, ideaIDStr string) error {
	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return err
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		return err
	}

	if idea.GroupID != uuid.Nil {
		if err := s.queries.DeleteIdeasByGroupID(ctx, idea.GroupID); err != nil {
			return err
		}
	} else {
		if err := s.queries.DeleteIdea(ctx, ideaID); err != nil {
			return err
		}
	}

	_ = s.queries.DeleteOrphanKeywords(ctx)
	return nil
}

type TraversalOptions struct {
	Max1HopPerNode int
	Max2HopPerNode int
	MaxTotalNodes  int
	MinWeight      float64
}

func DefaultTraversalOptions() TraversalOptions {
	return TraversalOptions{
		Max1HopPerNode: 4,
		Max2HopPerNode: 2,
		MaxTotalNodes:  10,
		MinWeight:      0.1,
	}
}

type TraversalNode struct {
	ID                  uuid.UUID
	Title               string
	Content             string
	RestructuredContent string
	Status              string
	HopDistance         int
	LinkedFromID        uuid.UUID
	RelationType        string
	Weight              float64
}

func (n TraversalNode) IsSeed() bool {
	return n.HopDistance == 0
}

type TraversalEdge struct {
	SourceID     uuid.UUID
	TargetID     uuid.UUID
	RelationType string
	Weight       float64
	HopLevel     int
}

type TraversalSubGraph struct {
	SeedIDs []uuid.UUID
	Nodes   []TraversalNode
	Edges   []TraversalEdge
}

func (s *IdeaService) TraverseSubGraph(ctx context.Context, seedIDs []uuid.UUID, opts TraversalOptions) (*TraversalSubGraph, error) {
	if opts.Max1HopPerNode <= 0 {
		opts.Max1HopPerNode = 4
	}
	if opts.Max2HopPerNode <= 0 {
		opts.Max2HopPerNode = 2
	}
	if opts.MaxTotalNodes <= 0 {
		opts.MaxTotalNodes = 10
	}

	result := &TraversalSubGraph{
		SeedIDs: make([]uuid.UUID, 0, len(seedIDs)),
		Nodes:   make([]TraversalNode, 0),
		Edges:   make([]TraversalEdge, 0),
	}

	if len(seedIDs) == 0 {
		return result, nil
	}

	visited := make(map[uuid.UUID]bool)
	seenEdges := make(map[string]bool)

	for _, seedID := range seedIDs {
		if seedID == uuid.Nil || visited[seedID] {
			continue
		}
		if len(result.Nodes) >= opts.MaxTotalNodes {
			break
		}

		idea, err := s.queries.GetIdeaByID(ctx, seedID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}

		visited[seedID] = true
		result.SeedIDs = append(result.SeedIDs, seedID)
		result.Nodes = append(result.Nodes, TraversalNode{
			ID:                  seedID,
			Title:               idea.Title,
			Content:             idea.Content,
			RestructuredContent: idea.RestructuredContent,
			Status:              idea.Status,
			HopDistance:         0,
			LinkedFromID:        uuid.Nil,
			RelationType:        "seed",
			Weight:              1.0,
		})
	}

	if len(result.Nodes) == 0 || len(result.Nodes) >= opts.MaxTotalNodes {
		return result, nil
	}

	var hop1IDs []uuid.UUID
	for _, seedID := range result.SeedIDs {
		if len(result.Nodes) >= opts.MaxTotalNodes {
			break
		}

		neighbors, err := s.queries.ListDirectConnectedIdeas(ctx, seedID)
		if err != nil {
			return nil, err
		}

		addedForSeed := 0
		for _, neighbor := range neighbors {
			if neighbor.Weight < opts.MinWeight {
				continue
			}

			aStr := seedID.String()
			bStr := neighbor.ID.String()
			if aStr > bStr {
				aStr, bStr = bStr, aStr
			}
			edgeKey := aStr + "<->" + bStr + ":" + neighbor.RelationType

			if !seenEdges[edgeKey] {
				seenEdges[edgeKey] = true
				result.Edges = append(result.Edges, TraversalEdge{
					SourceID:     seedID,
					TargetID:     neighbor.ID,
					RelationType: neighbor.RelationType,
					Weight:       neighbor.Weight,
					HopLevel:     1,
				})
			}

			if visited[neighbor.ID] {
				continue
			}
			if addedForSeed >= opts.Max1HopPerNode {
				continue
			}
			if len(result.Nodes) >= opts.MaxTotalNodes {
				break
			}

			visited[neighbor.ID] = true
			addedForSeed++
			hop1IDs = append(hop1IDs, neighbor.ID)

			result.Nodes = append(result.Nodes, TraversalNode{
				ID:                  neighbor.ID,
				Title:               neighbor.Title,
				Content:             neighbor.Content,
				RestructuredContent: neighbor.RestructuredContent,
				Status:              neighbor.Status,
				HopDistance:         1,
				LinkedFromID:        seedID,
				RelationType:        neighbor.RelationType,
				Weight:              neighbor.Weight,
			})
		}
	}

	if len(result.Nodes) >= opts.MaxTotalNodes || len(hop1IDs) == 0 {
		return result, nil
	}

	for _, hop1ID := range hop1IDs {
		if len(result.Nodes) >= opts.MaxTotalNodes {
			break
		}

		neighbors, err := s.queries.ListDirectConnectedIdeas(ctx, hop1ID)
		if err != nil {
			return nil, err
		}

		addedForHop1 := 0
		for _, neighbor := range neighbors {
			if neighbor.Weight < opts.MinWeight {
				continue
			}

			aStr := hop1ID.String()
			bStr := neighbor.ID.String()
			if aStr > bStr {
				aStr, bStr = bStr, aStr
			}
			edgeKey := aStr + "<->" + bStr + ":" + neighbor.RelationType

			if !seenEdges[edgeKey] {
				seenEdges[edgeKey] = true
				result.Edges = append(result.Edges, TraversalEdge{
					SourceID:     hop1ID,
					TargetID:     neighbor.ID,
					RelationType: neighbor.RelationType,
					Weight:       neighbor.Weight,
					HopLevel:     2,
				})
			}

			if visited[neighbor.ID] {
				continue
			}
			if addedForHop1 >= opts.Max2HopPerNode {
				continue
			}
			if len(result.Nodes) >= opts.MaxTotalNodes {
				break
			}

			visited[neighbor.ID] = true
			addedForHop1++

			result.Nodes = append(result.Nodes, TraversalNode{
				ID:                  neighbor.ID,
				Title:               neighbor.Title,
				Content:             neighbor.Content,
				RestructuredContent: neighbor.RestructuredContent,
				Status:              neighbor.Status,
				HopDistance:         2,
				LinkedFromID:        hop1ID,
				RelationType:        neighbor.RelationType,
				Weight:              neighbor.Weight,
			})
		}
	}

	return result, nil
}
