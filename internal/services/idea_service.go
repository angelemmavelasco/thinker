package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"thinker/internal/auth"
	"thinker/internal/database"
)

const (
	defaultSemanticThreshold = 0.70
	defaultSemanticTopK      = 5
)

type IdeaRolePermissionInfo struct {
	RoleID     uuid.UUID
	RoleName   string
	RoleSlug   string
	Permission string
}

type IdeaWithKeywords struct {
	database.Idea
	Keywords        []database.Keyword
	RolePermissions []IdeaRolePermissionInfo
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
	topicService   *TopicService
	jobQueue       chan ideaJob
	inFlight       sync.Map
	relinkQueue    chan uuid.UUID
	relinkInFlight sync.Map
}

func NewIdeaService(queries *database.Queries, llm *LLMService, embedding *EmbeddingService) *IdeaService {
	svc := &IdeaService{
		queries:        queries,
		llm:            llm,
		embedding:      embedding,
		keywordService: NewKeywordService(queries, embedding),
		topicService:   NewTopicService(queries, embedding, llm),
		jobQueue:       make(chan ideaJob, 100),
		relinkQueue:    make(chan uuid.UUID, 200),
	}
	go svc.startWorker()
	go svc.startRelinkWorker()
	return svc
}

func (s *IdeaService) TopicService() *TopicService {
	return s.topicService
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
			errStr := err.Error()
			if strings.Contains(errStr, "429") || strings.Contains(errStr, "1002") || strings.Contains(errStr, "rate limit") {
				time.Sleep(3 * time.Second)
			}
			continue
		}
		time.Sleep(350 * time.Millisecond)
	}
}

func (s *IdeaService) startRelinkWorker() {
	for ideaID := range s.relinkQueue {
		idKey := ideaID.String()
		if err := s.RelinkIdeaScope(context.Background(), ideaID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[relink] Error recalculando relaciones [%s]: %v", ideaID, err)
		}
		s.relinkInFlight.Delete(idKey)
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

func (s *IdeaService) EnqueueIdeaRelink(ideaID uuid.UUID) {
	if ideaID == uuid.Nil {
		return
	}
	idKey := ideaID.String()
	if _, loaded := s.relinkInFlight.LoadOrStore(idKey, true); loaded {
		return
	}
	select {
	case s.relinkQueue <- ideaID:
	default:
		go func() {
			s.relinkQueue <- ideaID
		}()
	}
}

func (s *IdeaService) EnqueueIdeasRelink(ideaIDs []uuid.UUID) {
	for _, id := range ideaIDs {
		s.EnqueueIdeaRelink(id)
	}
}

func (s *IdeaService) RelinkIdeaScope(ctx context.Context, ideaID uuid.UUID) error {
	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		return err
	}

	if err := s.queries.DeleteNonSequenceIdeaLinksByIdeaID(ctx, ideaID); err != nil {
		return err
	}

	if idea.Status != "procesada" && idea.Status != "processed" {
		return nil
	}

	if _, err := s.LinkOverlappingIdeas(ctx, ideaID); err != nil {
		return err
	}

	if strings.TrimSpace(idea.Embedding) != "" {
		if vec, err := DeserializeVector(idea.Embedding); err == nil && len(vec) > 0 {
			if _, err := s.LinkSemanticIdeas(ctx, ideaID, vec, defaultSemanticThreshold, defaultSemanticTopK); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *IdeaService) ListIdeas(ctx context.Context, actor Actor) ([]IdeaWithKeywords, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	var (
		ideas []database.Idea
		err   error
	)
	if actor.IsSuperuser {
		ideas, err = s.queries.ListIdeas(ctx)
	} else {
		ideas, err = s.queries.ListAccessibleIdeasByUser(ctx, actor.UserID)
	}
	if err != nil {
		return nil, err
	}

	permsByIdea := make(map[uuid.UUID][]IdeaRolePermissionInfo)
	if allPerms, permErr := s.queries.ListAllIdeaRolePermissions(ctx); permErr == nil {
		for _, p := range allPerms {
			permsByIdea[p.IdeaID] = append(permsByIdea[p.IdeaID], IdeaRolePermissionInfo{
				RoleID:     p.RoleID,
				RoleName:   p.RoleName,
				RoleSlug:   p.RoleSlug,
				Permission: p.Permission,
			})
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
			Idea:            idea,
			Keywords:        kws,
			RolePermissions: permsByIdea[idea.ID],
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

func (s *IdeaService) validateRolePermissionsMap(ctx context.Context, q *database.Queries, rolePerms map[uuid.UUID]string) (map[uuid.UUID]string, error) {
	validated := make(map[uuid.UUID]string, len(rolePerms))
	for roleID, rawPerm := range rolePerms {
		if roleID == uuid.Nil {
			continue
		}
		cleanPerm, ok := NormalizePermission(rawPerm)
		if !ok {
			return nil, ErrInvalidPermission
		}
		if _, err := q.GetRoleByID(ctx, roleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrRoleNotFound
			}
			return nil, err
		}
		validated[roleID] = cleanPerm
	}
	return validated, nil
}

func (s *IdeaService) CreateIdeasFromInput(ctx context.Context, actor Actor, title, content string, rolePerms map[uuid.UUID]string) ([]database.Idea, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	chunks := ChunkText(content, DefaultChunkMaxChars)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("el contenido está vacío")
	}

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	validatedPerms, err := s.validateRolePermissionsMap(ctx, qtx, rolePerms)
	if err != nil {
		return nil, err
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

		idea, err := qtx.CreateIdea(ctx, database.CreateIdeaParams{
			ID:         newID,
			UserID:     actor.UserID,
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

		_, _ = qtx.CreateIdeaChunk(ctx, database.CreateIdeaChunkParams{
			ID:         uuid.New(),
			IdeaID:     idea.ID,
			ChunkIndex: int32(i),
			Content:    chunk,
			Embedding:  "",
		})

		for roleID, perm := range validatedPerms {
			if _, err := qtx.UpsertIdeaRolePermission(ctx, database.UpsertIdeaRolePermissionParams{
				IdeaID:     idea.ID,
				RoleID:     roleID,
				Permission: perm,
				GrantedBy:  uuid.NullUUID{UUID: actor.UserID, Valid: true},
			}); err != nil {
				return nil, err
			}
		}

		created = append(created, idea)
	}

	for i := 1; i < len(created); i++ {
		prevID := created[i-1].ID
		currID := created[i].ID
		_, _ = qtx.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: prevID,
			TargetIdeaID: currID,
			RelationType: "sequence",
			Weight:       0.85,
		})
		_, _ = qtx.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: currID,
			TargetIdeaID: prevID,
			RelationType: "sequence",
			Weight:       0.85,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
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
	overlaps, err := s.queries.FindOverlappingIdeasInScope(ctx, ideaID)
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

	others, err := s.queries.ListIdeasWithEmbeddingsInScope(ctx, ideaID)
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
	if s.llm == nil {
		return nil, nil, nil, fmt.Errorf("servicio LLM no configurado")
	}
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

	if s.topicService != nil {
		if _, _, err := s.topicService.ResolveAndLinkIdea(ctx, ideaID, result); err != nil {
			log.Printf("[topics] Advertencia enlazando idea a temas [%s]: %v", ideaID, err)
		}
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
				existingPerms := make(map[uuid.UUID]string)
				if permRows, err := s.queries.ListIdeaRolePermissionsByIdeaID(ctx, idea.ID); err == nil {
					for _, row := range permRows {
						existingPerms[row.RoleID] = row.Permission
					}
				}
				ownerActor := Actor{UserID: idea.UserID}
				if _, err := s.CreateIdeasFromInput(ctx, ownerActor, idea.Title, idea.Content, existingPerms); err == nil {
					_ = s.queries.DeleteIdea(ctx, idea.ID)
					continue
				}
			}
			s.EnqueueIdeaAnalysis(idea, idea.Title, idea.Content)
		}
	}
}

func (s *IdeaService) GetNetworkGraph(ctx context.Context, actor Actor) (*GraphData, error) {
	ideasWithKws, err := s.ListIdeas(ctx, actor)
	if err != nil {
		return nil, err
	}

	rawLinks, err := s.queries.ListAllIdeaLinks(ctx)
	if err != nil {
		return nil, err
	}

	accessibleSet := make(map[uuid.UUID]bool, len(ideasWithKws))
	nodes := make([]GraphNode, 0, len(ideasWithKws))
	links := make([]GraphLink, 0, len(rawLinks))
	keywordNodes := make(map[string]GraphNode)

	for _, item := range ideasWithKws {
		accessibleSet[item.ID] = true
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

	if s.topicService != nil {
		edges, edgeErr := s.queries.ListAllHubIdeaEdges(ctx)
		allHubs, hubErr := s.topicService.ListAllHubs(ctx)
		allMOCs, mocErr := s.topicService.ListMOCs(ctx)

		if edgeErr == nil && hubErr == nil && mocErr == nil {
			visibleHubIDs := make(map[uuid.UUID]bool)
			filteredEdges := make([]database.HubIdeaEdge, 0)
			for _, e := range edges {
				if accessibleSet[e.IdeaID] {
					visibleHubIDs[e.HubID] = true
					filteredEdges = append(filteredEdges, e)
				}
			}

			visibleMocIDs := make(map[uuid.UUID]bool)
			visibleHubs := make([]database.KnowledgeHub, 0)
			for _, h := range allHubs {
				if actor.IsSuperuser || visibleHubIDs[h.ID] {
					visibleHubs = append(visibleHubs, h)
					visibleMocIDs[h.MocID] = true
				}
			}

			emittedMocIDs := make(map[uuid.UUID]bool)
			for _, m := range allMOCs {
				if actor.IsSuperuser || visibleMocIDs[m.ID] {
					emittedMocIDs[m.ID] = true
					nodes = append(nodes, GraphNode{
						ID:              "moc:" + m.ID.String(),
						Label:           m.Title,
						Type:            "moc",
						Content:         m.Description,
						OriginalContent: m.Code,
						Status:          "moc",
					})
				}
			}

			emittedHubIDs := make(map[uuid.UUID]bool)
			for _, h := range visibleHubs {
				hubID := "hub:" + h.ID.String()
				emittedHubIDs[h.ID] = true
				nodes = append(nodes, GraphNode{
					ID:              hubID,
					Label:           h.Title,
					Type:            "hub",
					Content:         h.Summary,
					OriginalContent: h.SearchAliases,
					Status:          "hub",
				})
				if emittedMocIDs[h.MocID] {
					links = append(links, GraphLink{
						Source: "moc:" + h.MocID.String(),
						Target: hubID,
						Type:   "contains_concept",
						Weight: 1.4,
					})
				}
			}

			for _, e := range filteredEdges {
				if emittedHubIDs[e.HubID] && accessibleSet[e.IdeaID] {
					links = append(links, GraphLink{
						Source: "hub:" + e.HubID.String(),
						Target: e.IdeaID.String(),
						Type:   e.RelationshipType,
						Weight: e.Weight,
					})
				}
			}
		}
	}

	seenPairs := make(map[string]struct{})
	for _, l := range rawLinks {
		if !accessibleSet[l.SourceIdeaID] || !accessibleSet[l.TargetIdeaID] {
			continue
		}
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
	Idea              database.Idea
	Keywords          []database.Keyword
	Connections       []ConnectedIdeaInfo
	GroupIdeas        []database.ListIdeasByGroupIDRow
	GroupCount        int
	CurrentPosition   int
	PrevFragmentID    *uuid.UUID
	NextFragmentID    *uuid.UUID
	RolePermissions   []IdeaRolePermissionInfo
	RolePermissionMap map[string]string
	CanEdit           bool
	CanDelete         bool
	CanManageRoles    bool
	AuthorDisplay     string
	UpdatedByDisplay  string
}

func (s *IdeaService) GetIdeaDetail(ctx context.Context, actor Actor, ideaIDStr string) (*IdeaDetail, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdeaNotFound
		}
		return nil, err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, ErrIdeaNotFound
	}

	canEdit, err := CanEditIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}

	kws, err := s.keywordService.ListKeywordsByIdeaID(ctx, ideaID)
	if err != nil {
		return nil, err
	}

	var connections []ConnectedIdeaInfo
	indexByID := make(map[uuid.UUID]int)

	if actor.IsSuperuser {
		rawLinks, err := s.queries.ListDirectConnectedIdeas(ctx, ideaID)
		if err != nil {
			return nil, err
		}
		for _, row := range rawLinks {
			appendConnection(&connections, indexByID, row.ID, row.Title, row.Content, row.RestructuredContent, row.Status, row.RelationType)
		}
	} else {
		rawLinks, err := s.queries.ListAccessibleDirectConnectedIdeas(ctx, database.ListAccessibleDirectConnectedIdeasParams{
			SourceIdeaID: ideaID,
			UserID:       actor.UserID,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rawLinks {
			appendConnection(&connections, indexByID, row.ID, row.Title, row.Content, row.RestructuredContent, row.Status, row.RelationType)
		}
	}

	permRows, _ := s.queries.ListIdeaRolePermissionsByIdeaID(ctx, ideaID)
	rolePerms := make([]IdeaRolePermissionInfo, 0, len(permRows))
	rolePermMap := make(map[string]string, len(permRows))
	for _, row := range permRows {
		rolePerms = append(rolePerms, IdeaRolePermissionInfo{
			RoleID:     row.RoleID,
			RoleName:   row.RoleName,
			RoleSlug:   row.RoleSlug,
			Permission: row.Permission,
		})
		rolePermMap[row.RoleID.String()] = row.Permission
	}

	authorDisplay := idea.UserID.String()
	if authorUser, userErr := s.queries.GetUserByID(ctx, idea.UserID); userErr == nil {
		display, _ := auth.UserDisplayName("", "", authorUser.Email)
		if display != "" {
			authorDisplay = display
		}
	}

	updatedByDisplay := ""
	if idea.UpdatedBy.Valid {
		if editorUser, userErr := s.queries.GetUserByID(ctx, idea.UpdatedBy.UUID); userErr == nil {
			display, _ := auth.UserDisplayName("", "", editorUser.Email)
			updatedByDisplay = display
		}
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
		Idea:              idea,
		Keywords:          kws,
		Connections:       connections,
		GroupIdeas:        groupIdeas,
		GroupCount:        groupCount,
		CurrentPosition:   currentPos,
		PrevFragmentID:    prevID,
		NextFragmentID:    nextID,
		RolePermissions:   rolePerms,
		RolePermissionMap: rolePermMap,
		CanEdit:           canEdit,
		CanDelete:         actor.CanDeleteIdea(idea),
		CanManageRoles:    actor.CanManageIdeaPermissions(idea),
		AuthorDisplay:     authorDisplay,
		UpdatedByDisplay:  updatedByDisplay,
	}, nil
}

func appendConnection(connections *[]ConnectedIdeaInfo, indexByID map[uuid.UUID]int, id uuid.UUID, title, content, restructured, status, relType string) {
	relLabel := "Ideas afines"
	if relType == "concept_overlap" {
		relLabel = "Temas en común"
	} else if relType == "sequence" {
		relLabel = "Secuencia"
	}

	if idx, exists := indexByID[id]; exists {
		if (*connections)[idx].RelationDescription != relLabel {
			(*connections)[idx].RelationDescription = "Afinidad y temas en común"
		}
		return
	}

	preview := content
	if restructured != "" {
		preview = restructured
	}

	indexByID[id] = len(*connections)
	*connections = append(*connections, ConnectedIdeaInfo{
		ID:                  id,
		Title:               title,
		Preview:             preview,
		Status:              status,
		RelationDescription: relLabel,
	})
}

func (s *IdeaService) UpdateIdeaPermissions(ctx context.Context, actor Actor, ideaIDStr string, rolePerms map[uuid.UUID]string) (*IdeaDetail, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdeaNotFound
		}
		return nil, err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, ErrIdeaNotFound
	}

	if !actor.CanManageIdeaPermissions(idea) {
		return nil, ErrForbidden
	}

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	validatedPerms, err := s.validateRolePermissionsMap(ctx, qtx, rolePerms)
	if err != nil {
		return nil, err
	}

	targetIdeaIDs := []uuid.UUID{idea.ID}
	if idea.GroupID != uuid.Nil {
		if groupRows, groupErr := qtx.ListIdeasByGroupID(ctx, idea.GroupID); groupErr == nil && len(groupRows) > 0 {
			targetIdeaIDs = make([]uuid.UUID, 0, len(groupRows))
			for _, row := range groupRows {
				targetIdeaIDs = append(targetIdeaIDs, row.ID)
			}
		}
	}

	for _, tid := range targetIdeaIDs {
		if err := qtx.DeleteIdeaRolePermissionsByIdeaID(ctx, tid); err != nil {
			return nil, err
		}
		for roleID, perm := range validatedPerms {
			if _, err := qtx.UpsertIdeaRolePermission(ctx, database.UpsertIdeaRolePermissionParams{
				IdeaID:     tid,
				RoleID:     roleID,
				Permission: perm,
				GrantedBy:  uuid.NullUUID{UUID: actor.UserID, Valid: true},
			}); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if len(targetIdeaIDs) <= 10 {
		for _, tid := range targetIdeaIDs {
			_ = s.RelinkIdeaScope(ctx, tid)
		}
	} else {
		_ = s.RelinkIdeaScope(ctx, idea.ID)
		for _, tid := range targetIdeaIDs {
			if tid != idea.ID {
				s.EnqueueIdeaRelink(tid)
			}
		}
	}

	return s.GetIdeaDetail(ctx, actor, ideaIDStr)
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
			RelationType: "sequence",
			Weight:       0.85,
		})
		_, _ = s.queries.CreateIdeaLink(ctx, database.CreateIdeaLinkParams{
			ID:           uuid.New(),
			SourceIdeaID: currID,
			TargetIdeaID: prevID,
			RelationType: "sequence",
			Weight:       0.85,
		})
	}
}

func (s *IdeaService) UpdateIdea(ctx context.Context, actor Actor, ideaIDStr, title, content string) (*IdeaDetail, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdeaNotFound
		}
		return nil, err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, ErrIdeaNotFound
	}

	canEdit, err := CanEditIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canEdit {
		return nil, ErrForbidden
	}

	_ = s.queries.DeleteIdeaLinksByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteIdeaKeywordsByIdeaID(ctx, ideaID)
	_ = s.queries.DeleteOrphanKeywords(ctx)

	updated, err := s.queries.UpdateIdeaContentAndReset(ctx, database.UpdateIdeaContentAndResetParams{
		ID:        ideaID,
		Title:     title,
		Content:   content,
		UpdatedBy: uuid.NullUUID{UUID: actor.UserID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	s.relinkGroupSequence(ctx, updated.GroupID)
	s.EnqueueIdeaAnalysis(updated, updated.Title, updated.Content)

	return s.GetIdeaDetail(ctx, actor, ideaIDStr)
}

func (s *IdeaService) ReanalyzeIdea(ctx context.Context, actor Actor, ideaIDStr string) (*IdeaDetail, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return nil, ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdeaNotFound
		}
		return nil, err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, ErrIdeaNotFound
	}

	canEdit, err := CanEditIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return nil, err
	}
	if !canEdit {
		return nil, ErrForbidden
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

	return s.GetIdeaDetail(ctx, actor, ideaIDStr)
}

func (s *IdeaService) DeleteIdeaFragment(ctx context.Context, actor Actor, ideaIDStr string) error {
	if !actor.IsValid() {
		return ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIdeaNotFound
		}
		return err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return err
	}
	if !canView {
		return ErrIdeaNotFound
	}

	if !actor.CanDeleteIdea(idea) {
		return ErrForbidden
	}

	if err := s.queries.DeleteIdea(ctx, ideaID); err != nil {
		return err
	}

	_ = s.queries.DeleteOrphanKeywords(ctx)
	s.relinkGroupSequence(ctx, idea.GroupID)
	return nil
}

func (s *IdeaService) DeleteIdeaGroup(ctx context.Context, actor Actor, ideaIDStr string) error {
	if !actor.IsValid() {
		return ErrUnauthorized
	}

	ideaID, err := uuid.Parse(ideaIDStr)
	if err != nil {
		return ErrIdeaNotFound
	}

	idea, err := s.queries.GetIdeaByID(ctx, ideaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIdeaNotFound
		}
		return err
	}

	canView, err := CanViewIdea(ctx, s.queries, actor, idea)
	if err != nil {
		return err
	}
	if !canView {
		return ErrIdeaNotFound
	}

	if !actor.CanDeleteIdea(idea) {
		return ErrForbidden
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
		Max1HopPerNode: 3,
		Max2HopPerNode: 2,
		MaxTotalNodes:  6,
		MinWeight:      0.60,
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

func (s *IdeaService) TraverseSubGraph(ctx context.Context, actor Actor, seedIDs []uuid.UUID, opts TraversalOptions) (*TraversalSubGraph, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

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

	accessibleSet, err := ListAccessibleIdeaIDSet(ctx, s.queries, actor)
	if err != nil {
		return nil, err
	}
	if len(accessibleSet) == 0 {
		return result, nil
	}

	visited := make(map[uuid.UUID]bool)
	seenEdges := make(map[string]bool)

	for _, seedID := range seedIDs {
		if seedID == uuid.Nil || visited[seedID] || !accessibleSet[seedID] {
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
			if !accessibleSet[neighbor.ID] {
				continue
			}
			if neighbor.RelationType != "sequence" && neighbor.Weight < opts.MinWeight {
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
			if !accessibleSet[neighbor.ID] {
				continue
			}
			if neighbor.RelationType != "sequence" && neighbor.Weight < opts.MinWeight {
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

type SeedCandidate struct {
	IdeaID       uuid.UUID
	Title        string
	Score        float64
	VectorScore  float64
	KeywordScore float64
	MatchedTerms []string
}

func extractQueryWords(raw string) []string {
	stopWords := map[string]bool{
		"el": true, "la": true, "los": true, "las": true,
		"un": true, "una": true, "unos": true, "unas": true,
		"de": true, "del": true, "en": true, "para": true,
		"por": true, "con": true, "que": true, "como": true,
		"sobre": true, "entre": true, "este": true, "esta": true,
		"estos": true, "estas": true, "a": true, "al": true,
		"y": true, "o": true, "u": true, "the": true,
		"is": true, "are": true, "was": true, "were": true,
		"in": true, "on": true, "at": true, "to": true,
		"for": true, "with": true, "about": true, "of": true,
	}

	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, raw)

	fields := strings.Fields(cleaned)
	var words []string
	seen := make(map[string]bool)

	for _, f := range fields {
		if len([]rune(f)) < 3 || stopWords[f] || seen[f] {
			continue
		}
		seen[f] = true
		words = append(words, f)
	}

	if len(words) == 0 && len(fields) > 0 {
		for _, f := range fields {
			if !seen[f] {
				seen[f] = true
				words = append(words, f)
			}
		}
	}

	return words
}

const (
	DefaultMinSeedScore    = 0.45
	DefaultMinVectorScore  = 0.40
	DefaultMinKeywordScore = 0.20
)

func (s *IdeaService) FindSeedIdeas(ctx context.Context, actor Actor, queryText string, topK int) ([]SeedCandidate, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	cleanedQuery := strings.TrimSpace(queryText)
	if cleanedQuery == "" {
		return nil, nil
	}

	if topK <= 0 {
		topK = 3
	}

	var (
		ideas []database.Idea
		err   error
	)
	if actor.IsSuperuser {
		ideas, err = s.queries.ListIdeas(ctx)
	} else {
		ideas, err = s.queries.ListAccessibleIdeasByUser(ctx, actor.UserID)
	}
	if err != nil {
		return nil, err
	}
	if len(ideas) == 0 {
		return nil, nil
	}

	searchQuery := cleanedQuery
	if s.llm != nil {
		if expanded, err := s.llm.ExpandQuery(ctx, cleanedQuery); err == nil && strings.TrimSpace(expanded) != "" {
			searchQuery = strings.TrimSpace(expanded)
		}
	}

	var queryVec []float64
	if s.embedding != nil {
		if vec, err := s.embedding.GenerateEmbedding(ctx, searchQuery); err == nil && len(vec) > 0 {
			queryVec = vec
		} else if vec, err := s.embedding.GenerateEmbedding(ctx, cleanedQuery); err == nil && len(vec) > 0 {
			queryVec = vec
		}
	}

	rawWords := extractQueryWords(cleanedQuery)
	queryWords := extractQueryWords(searchQuery)
	for _, w := range rawWords {
		found := false
		for _, qw := range queryWords {
			if qw == w {
				found = true
				break
			}
		}
		if !found {
			queryWords = append(queryWords, w)
		}
	}

	lowerQuery := strings.ToLower(searchQuery + " " + cleanedQuery)

	candidates := make([]SeedCandidate, 0, len(ideas))

	for _, idea := range ideas {
		var vecScore float64
		if len(queryVec) > 0 && idea.Embedding != "" {
			if ideaVec, err := DeserializeVector(idea.Embedding); err == nil && len(ideaVec) > 0 {
				vecScore = CosineSimilarity(queryVec, ideaVec)
				if vecScore < 0 {
					vecScore = 0
				}
			}
		}

		var matchedTerms []string
		seenMatched := make(map[string]bool)

		lowerTitle := strings.ToLower(idea.Title)
		for _, w := range queryWords {
			if strings.Contains(lowerTitle, w) && !seenMatched[w] {
				seenMatched[w] = true
				matchedTerms = append(matchedTerms, w)
			}
		}

		if s.keywordService != nil {
			if kws, err := s.keywordService.ListKeywordsByIdeaID(ctx, idea.ID); err == nil {
				for _, kw := range kws {
					kwLower := strings.ToLower(kw.Name)
					if strings.Contains(lowerQuery, kwLower) && !seenMatched[kwLower] {
						seenMatched[kwLower] = true
						matchedTerms = append(matchedTerms, kwLower)
					} else {
						for _, w := range queryWords {
							if strings.Contains(kwLower, w) && !seenMatched[w] {
								seenMatched[w] = true
								matchedTerms = append(matchedTerms, w)
							}
						}
					}
				}
			}
		}

		lowerContent := strings.ToLower(idea.Content)
		for _, w := range queryWords {
			if strings.Contains(lowerContent, w) && !seenMatched[w] {
				seenMatched[w] = true
				matchedTerms = append(matchedTerms, w)
			}
		}

		var kwScore float64
		if len(rawWords) > 0 {
			rawMatches := 0
			for _, rw := range rawWords {
				if seenMatched[rw] {
					rawMatches++
				}
			}
			rawRatio := float64(rawMatches) / float64(len(rawWords))
			var expRatio float64
			if len(queryWords) > 0 {
				expRatio = float64(len(matchedTerms)) / float64(len(queryWords))
			}
			if rawRatio > expRatio {
				kwScore = rawRatio
			} else {
				kwScore = expRatio
			}
		} else if len(queryWords) > 0 {
			kwScore = float64(len(matchedTerms)) / float64(len(queryWords))
		}
		if kwScore > 1.0 {
			kwScore = 1.0
		}

		var finalScore float64
		if len(queryVec) > 0 && len(queryWords) > 0 {
			if vecScore < DefaultMinVectorScore && kwScore < DefaultMinKeywordScore {
				continue
			}
			if kwScore >= 0.35 {
				finalScore = (0.50 * vecScore) + (0.50 * kwScore)
			} else {
				finalScore = (0.75 * vecScore) + (0.25 * kwScore)
			}
			if finalScore < DefaultMinSeedScore && kwScore < 0.40 {
				continue
			}
			if finalScore < DefaultMinSeedScore {
				finalScore = DefaultMinSeedScore
			}
		} else if len(queryVec) > 0 {
			if vecScore < DefaultMinSeedScore {
				continue
			}
			finalScore = vecScore
		} else {
			if kwScore < DefaultMinKeywordScore {
				continue
			}
			finalScore = kwScore
		}

		candidates = append(candidates, SeedCandidate{
			IdeaID:       idea.ID,
			Title:        idea.Title,
			Score:        finalScore,
			VectorScore:  vecScore,
			KeywordScore: kwScore,
			MatchedTerms: matchedTerms,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	return candidates, nil
}

func (s *IdeaService) FindHierarchicalSeeds(ctx context.Context, actor Actor, queryText string, topK int) ([]SeedCandidate, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	if s.topicService == nil || s.llm == nil {
		return nil, nil
	}

	cleanedQuery := strings.TrimSpace(queryText)
	if cleanedQuery == "" {
		return nil, nil
	}

	accessibleSet, err := ListAccessibleIdeaIDSet(ctx, s.queries, actor)
	if err != nil {
		return nil, err
	}
	if len(accessibleSet) == 0 {
		return nil, nil
	}

	allEdges, err := s.queries.ListAllHubIdeaEdges(ctx)
	if err != nil {
		return nil, nil
	}

	visibleHubIDs := make(map[uuid.UUID]bool)
	for _, e := range allEdges {
		if accessibleSet[e.IdeaID] {
			visibleHubIDs[e.HubID] = true
		}
	}
	if len(visibleHubIDs) == 0 {
		return nil, nil
	}

	allHubs, err := s.topicService.ListAllHubs(ctx)
	if err != nil || len(allHubs) == 0 {
		return nil, nil
	}

	visibleHubsByMOC := make(map[uuid.UUID][]database.KnowledgeHub)
	visibleMocIDs := make(map[uuid.UUID]bool)
	for _, h := range allHubs {
		if visibleHubIDs[h.ID] {
			visibleHubsByMOC[h.MocID] = append(visibleHubsByMOC[h.MocID], h)
			visibleMocIDs[h.MocID] = true
		}
	}
	if len(visibleMocIDs) == 0 {
		return nil, nil
	}

	allMOCs, err := s.topicService.ListMOCs(ctx)
	if err != nil || len(allMOCs) == 0 {
		return nil, nil
	}

	descriptors := make([]MOCDescriptor, 0, len(visibleMocIDs))
	mocByCode := make(map[string]database.KnowledgeMoc, len(visibleMocIDs))
	for _, m := range allMOCs {
		if !visibleMocIDs[m.ID] {
			continue
		}
		descriptors = append(descriptors, MOCDescriptor{
			Code:        m.Code,
			Title:       m.Title,
			Description: m.Description,
		})
		mocByCode[strings.ToUpper(strings.TrimSpace(m.Code))] = m
	}

	if len(descriptors) == 0 {
		return nil, nil
	}

	targetCodes, err := s.llm.TriageQueryMOC(ctx, cleanedQuery, descriptors)
	if err != nil || len(targetCodes) == 0 {
		return nil, nil
	}

	var queryVec []float64
	if s.embedding != nil {
		if vec, err := s.embedding.GenerateEmbedding(ctx, cleanedQuery); err == nil && len(vec) > 0 {
			queryVec = vec
		}
	}

	queryWords := extractQueryWords(cleanedQuery)
	lowerQuery := strings.ToLower(cleanedQuery)

	type scoredHub struct {
		hub   database.KnowledgeHub
		score float64
	}
	var candidateHubs []scoredHub

	for _, code := range targetCodes {
		moc, exists := mocByCode[strings.ToUpper(code)]
		if !exists {
			continue
		}

		hubs := visibleHubsByMOC[moc.ID]
		if len(hubs) == 0 {
			continue
		}

		for _, h := range hubs {
			var vecScore float64
			if len(queryVec) > 0 && h.Embedding != "" {
				if hubVec, err := DeserializeVector(h.Embedding); err == nil && len(hubVec) > 0 {
					vecScore = CosineSimilarity(queryVec, hubVec)
					if vecScore < 0 {
						vecScore = 0
					}
				}
			}

			aliasScore := 0.0
			lowerTitle := strings.ToLower(h.Title)
			if strings.Contains(lowerQuery, lowerTitle) {
				aliasScore += 0.5
			}
			for _, w := range queryWords {
				if strings.Contains(lowerTitle, w) {
					aliasScore += 0.25
				}
			}

			aliases := parseAliases(h.SearchAliases)
			for _, a := range aliases {
				lowerA := strings.ToLower(a)
				if strings.Contains(lowerQuery, lowerA) {
					aliasScore += 0.5
					break
				}
				for _, w := range queryWords {
					if strings.Contains(lowerA, w) {
						aliasScore += 0.25
						break
					}
				}
			}
			if aliasScore > 1.0 {
				aliasScore = 1.0
			}

			hubScore := (0.60 * vecScore) + (0.40 * aliasScore)
			if hubScore > 0.30 || aliasScore >= 0.40 {
				candidateHubs = append(candidateHubs, scoredHub{
					hub:   h,
					score: hubScore,
				})
			}
		}
	}

	if len(candidateHubs) == 0 {
		return nil, nil
	}

	sort.Slice(candidateHubs, func(i, j int) bool {
		return candidateHubs[i].score > candidateHubs[j].score
	})

	if len(candidateHubs) > 3 {
		candidateHubs = candidateHubs[:3]
	}

	seenIdeaIDs := make(map[uuid.UUID]bool)
	var candidates []SeedCandidate

	for _, ch := range candidateHubs {
		linkedIdeas, err := s.topicService.ListIdeasByHub(ctx, ch.hub.ID)
		if err != nil || len(linkedIdeas) == 0 {
			continue
		}

		for _, row := range linkedIdeas {
			if !accessibleSet[row.ID] || seenIdeaIDs[row.ID] {
				continue
			}
			seenIdeaIDs[row.ID] = true

			finalScore := ch.score * row.Weight
			if finalScore < 0.25 {
				finalScore = 0.25
			}
			if finalScore > 1.0 {
				finalScore = 1.0
			}

			candidates = append(candidates, SeedCandidate{
				IdeaID:       row.ID,
				Title:        row.Title,
				Score:        finalScore,
				VectorScore:  ch.score,
				KeywordScore: row.Weight,
				MatchedTerms: []string{ch.hub.Title},
			})
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if topK <= 0 {
		topK = 3
	}
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	return candidates, nil
}

func (s *IdeaService) SearchAndExpand(ctx context.Context, actor Actor, queryText string, topKSeeds int, opts TraversalOptions) (*TraversalSubGraph, []SeedCandidate, error) {
	if !actor.IsValid() {
		return nil, nil, ErrUnauthorized
	}

	var seeds []SeedCandidate
	if s.topicService != nil && s.llm != nil {
		hSeeds, hErr := s.FindHierarchicalSeeds(ctx, actor, queryText, topKSeeds)
		if hErr == nil && len(hSeeds) > 0 {
			seeds = hSeeds
		}
	}

	if len(seeds) == 0 {
		var err error
		seeds, err = s.FindSeedIdeas(ctx, actor, queryText, topKSeeds)
		if err != nil {
			return nil, nil, err
		}
	}

	if len(seeds) == 0 {
		return &TraversalSubGraph{
			SeedIDs: make([]uuid.UUID, 0),
			Nodes:   make([]TraversalNode, 0),
			Edges:   make([]TraversalEdge, 0),
		}, nil, nil
	}

	seedIDs := make([]uuid.UUID, 0, len(seeds))
	for _, seed := range seeds {
		seedIDs = append(seedIDs, seed.IdeaID)
	}

	subGraph, err := s.TraverseSubGraph(ctx, actor, seedIDs, opts)
	if err != nil {
		return nil, nil, err
	}

	return subGraph, seeds, nil
}

func BuildSubGraphPrompt(subGraph *TraversalSubGraph) string {
	if subGraph == nil || len(subGraph.Nodes) == 0 {
		return ""
	}

	titleByID := make(map[uuid.UUID]string, len(subGraph.Nodes))
	for _, node := range subGraph.Nodes {
		titleByID[node.ID] = node.Title
	}

	var sb strings.Builder

	sb.WriteString("Contexto primario (Nodos Semilla):\n")
	hasSeeds := false
	for _, node := range subGraph.Nodes {
		if node.HopDistance == 0 {
			hasSeeds = true
			content := node.Content
			if strings.TrimSpace(content) == "" {
				content = node.RestructuredContent
			}
			sb.WriteString(fmt.Sprintf("- [%s]: %s\n", node.Title, strings.TrimSpace(content)))
		}
	}
	if !hasSeeds {
		sb.WriteString("(Sin nodos semilla directos)\n")
	}

	sb.WriteString("\nContexto asociativo conectado (Vecinos directos y secundarios):\n")
	hasNeighbors := false
	for _, node := range subGraph.Nodes {
		if node.HopDistance > 0 {
			hasNeighbors = true
			content := node.Content
			if strings.TrimSpace(content) == "" {
				content = node.RestructuredContent
			}
			parentTitle := titleByID[node.LinkedFromID]
			if parentTitle == "" {
				parentTitle = "Nodo inicial"
			}
			sb.WriteString(fmt.Sprintf("- [%s] (conectada con %s por '%s'): %s\n",
				node.Title, parentTitle, node.RelationType, strings.TrimSpace(content)))
		}
	}
	if !hasNeighbors {
		sb.WriteString("(Sin vecinos conectados adicionales)\n")
	}

	return strings.TrimSpace(sb.String())
}

type QueryResult struct {
	Question      string
	Answer        string
	SubGraph      *TraversalSubGraph
	Seeds         []SeedCandidate
	ContextPrompt string
}

func (s *IdeaService) QueryGraph(ctx context.Context, actor Actor, userQuery string) (*QueryResult, error) {
	if !actor.IsValid() {
		return nil, ErrUnauthorized
	}

	cleanedQuery := strings.TrimSpace(userQuery)
	if cleanedQuery == "" {
		return nil, fmt.Errorf("la consulta está vacía")
	}

	opts := DefaultTraversalOptions()
	subGraph, seeds, err := s.SearchAndExpand(ctx, actor, cleanedQuery, 3, opts)
	if err != nil {
		return nil, err
	}

	contextPrompt := BuildSubGraphPrompt(subGraph)

	var answer string
	if s.llm != nil {
		ans, err := s.llm.AskKnowledgeGraph(ctx, cleanedQuery, subGraph)
		if err != nil {
			return nil, err
		}
		answer = ans
	}

	return &QueryResult{
		Question:      cleanedQuery,
		Answer:        answer,
		SubGraph:      subGraph,
		Seeds:         seeds,
		ContextPrompt: contextPrompt,
	}, nil
}
