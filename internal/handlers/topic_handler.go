package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/config"
	"thinker/internal/services"
	"thinker/internal/urls"
)

type TopicHandler struct {
	topicService *services.TopicService
	ideaService  *services.IdeaService
}

func NewTopicHandler(topicService *services.TopicService, ideaService *services.IdeaService) *TopicHandler {
	return &TopicHandler{
		topicService: topicService,
		ideaService:  ideaService,
	}
}

type TopicIdeaViewItem struct {
	ID               string
	Title            string
	RelationType     string
	Weight           float64
	IsManualOverride bool
}

type TopicHubViewItem struct {
	ID            string
	MocID         string
	Title         string
	Summary       string
	SearchAliases []string
	IsLocked      bool
	DisplayOrder  int32
	Ideas         []TopicIdeaViewItem
}

type TopicMOCViewItem struct {
	ID           string
	Code         string
	Title        string
	Description  string
	IsLocked     bool
	DisplayOrder int32
	Hubs         []TopicHubViewItem
}

type HubOptionItem struct {
	ID       string
	Title    string
	MocCode  string
	MocTitle string
}

func (h *TopicHandler) ListTopics(c *gin.Context) {
	ctx := c.Request.Context()

	rawMOCs, err := h.topicService.ListMOCs(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error consultando temas: %v", err)
		return
	}

	totalHubs := 0
	totalIdeasLinked := 0
	mocsView := make([]TopicMOCViewItem, 0, len(rawMOCs))
	allHubOptions := make([]HubOptionItem, 0)

	for _, m := range rawMOCs {
		rawHubs, err := h.topicService.ListHubsByMOC(ctx, m.ID)
		if err != nil {
			continue
		}

		hubsView := make([]TopicHubViewItem, 0, len(rawHubs))
		for _, hub := range rawHubs {
			totalHubs++

			var aliases []string
			if strings.TrimSpace(hub.SearchAliases) != "" {
				_ = json.Unmarshal([]byte(hub.SearchAliases), &aliases)
			}

			rawIdeas, _ := h.topicService.ListIdeasByHub(ctx, hub.ID)
			ideasView := make([]TopicIdeaViewItem, 0, len(rawIdeas))
			for _, idea := range rawIdeas {
				totalIdeasLinked++
				ideasView = append(ideasView, TopicIdeaViewItem{
					ID:               idea.ID.String(),
					Title:            idea.Title,
					RelationType:     idea.RelationshipType,
					Weight:           idea.Weight,
					IsManualOverride: idea.IsManualOverride == 1,
				})
			}

			hubsView = append(hubsView, TopicHubViewItem{
				ID:            hub.ID.String(),
				MocID:         hub.MocID.String(),
				Title:         hub.Title,
				Summary:       hub.Summary,
				SearchAliases: aliases,
				IsLocked:      hub.IsLocked == 1,
				DisplayOrder:  hub.DisplayOrder,
				Ideas:         ideasView,
			})

			allHubOptions = append(allHubOptions, HubOptionItem{
				ID:       hub.ID.String(),
				Title:    hub.Title,
				MocCode:  m.Code,
				MocTitle: m.Title,
			})
		}

		mocsView = append(mocsView, TopicMOCViewItem{
			ID:           m.ID.String(),
			Code:         m.Code,
			Title:        m.Title,
			Description:  m.Description,
			IsLocked:     m.IsLocked == 1,
			DisplayOrder: m.DisplayOrder,
			Hubs:         hubsView,
		})
	}

	allowAuto := h.topicService.GetBoolSetting(ctx, "allow_auto_moc_creation", true)

	data := gin.H{
		"title":              "Temas e índices",
		"mocs":               mocsView,
		"hub_options":        allHubOptions,
		"allow_auto_mocs":    allowAuto,
		"total_mocs":         len(mocsView),
		"total_hubs":         totalHubs,
		"total_ideas_linked": totalIdeasLinked,
	}

	config.Render(c, http.StatusOK, "topics_index.html", data)
}

func (h *TopicHandler) CreateMOC(c *gin.Context) {
	ctx := c.Request.Context()
	code := strings.TrimSpace(c.PostForm("code"))
	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))

	if title == "" {
		c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
		return
	}

	if code == "" {
		clean := strings.ToUpper(strings.ReplaceAll(title, " ", "_"))
		var b strings.Builder
		for _, r := range clean {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				b.WriteRune(r)
			}
		}
		code = b.String()
		if code == "" {
			code = "TEMA"
		}
	}

	_, _ = h.topicService.CreateMOC(ctx, code, title, description, true)
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) UpdateMOC(c *gin.Context) {
	ctx := c.Request.Context()
	mocIDStr := c.Param("id")
	mocID, err := uuid.Parse(mocIDStr)
	if err != nil {
		c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))
	displayOrder, _ := strconv.Atoi(c.PostForm("display_order"))

	_, _ = h.topicService.UpdateMOC(ctx, mocID, title, description, int32(displayOrder), true)
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) DeleteMOC(c *gin.Context) {
	ctx := c.Request.Context()
	mocIDStr := c.Param("id")
	if mocID, err := uuid.Parse(mocIDStr); err == nil {
		_ = h.topicService.DeleteMOC(ctx, mocID)
	}
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) ToggleMOCLock(c *gin.Context) {
	ctx := c.Request.Context()
	mocIDStr := c.Param("id")
	if mocID, err := uuid.Parse(mocIDStr); err == nil {
		if moc, err := h.topicService.GetMOCByID(ctx, mocID); err == nil {
			newLocked := moc.IsLocked == 0
			_, _ = h.topicService.UpdateMOC(ctx, moc.ID, moc.Title, moc.Description, moc.DisplayOrder, newLocked)
		}
	}
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) CreateHub(c *gin.Context) {
	ctx := c.Request.Context()
	mocIDStr := c.PostForm("moc_id")
	mocID, err := uuid.Parse(mocIDStr)
	if err != nil {
		c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	summary := strings.TrimSpace(c.PostForm("summary"))
	rawAliases := strings.TrimSpace(c.PostForm("search_aliases"))

	var aliases []string
	if rawAliases != "" {
		parts := strings.Split(rawAliases, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				aliases = append(aliases, trimmed)
			}
		}
	}

	_, _ = h.topicService.CreateHub(ctx, mocID, title, summary, aliases, true)
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) UpdateHub(c *gin.Context) {
	ctx := c.Request.Context()
	hubIDStr := c.Param("id")
	hubID, err := uuid.Parse(hubIDStr)
	if err != nil {
		c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
		return
	}

	hub, err := h.topicService.GetHubByID(ctx, hubID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	summary := strings.TrimSpace(c.PostForm("summary"))
	rawAliases := strings.TrimSpace(c.PostForm("search_aliases"))
	displayOrder, _ := strconv.Atoi(c.PostForm("display_order"))

	var aliases []string
	if rawAliases != "" {
		parts := strings.Split(rawAliases, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				aliases = append(aliases, trimmed)
			}
		}
	}

	_, _ = h.topicService.UpdateHub(ctx, hubID, hub.MocID, title, summary, aliases, int32(displayOrder), true)
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) DeleteHub(c *gin.Context) {
	ctx := c.Request.Context()
	hubIDStr := c.Param("id")
	if hubID, err := uuid.Parse(hubIDStr); err == nil {
		_ = h.topicService.DeleteHub(ctx, hubID)
	}
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) MoveIdea(c *gin.Context) {
	ctx := c.Request.Context()
	ideaIDStr := strings.TrimSpace(c.PostForm("idea_id"))
	targetHubIDStr := strings.TrimSpace(c.PostForm("target_hub_id"))

	ideaID, err1 := uuid.Parse(ideaIDStr)
	targetHubID, err2 := uuid.Parse(targetHubIDStr)

	if err1 == nil && err2 == nil {
		if hub, err := h.topicService.GetHubByID(ctx, targetHubID); err == nil {
			_ = h.topicService.MoveIdeaToHub(ctx, ideaID, hub.MocID, targetHubID)
		}
	}

	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}

func (h *TopicHandler) ToggleAutoMOCSetting(c *gin.Context) {
	ctx := c.Request.Context()
	current := h.topicService.GetBoolSetting(ctx, "allow_auto_moc_creation", true)
	newVal := "false"
	if !current {
		newVal = "true"
	}
	_ = h.topicService.SetSetting(ctx, "allow_auto_moc_creation", newVal)
	c.Redirect(http.StatusSeeOther, urls.Get("topics:list"))
}
