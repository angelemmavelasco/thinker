package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/services"
)

type IdeaHandler struct {
	service *services.IdeaService
}

func NewIdeaHandler(queries *database.Queries, llm *services.LLMService, embedding *services.EmbeddingService) *IdeaHandler {
	svc := services.NewIdeaService(queries, llm, embedding)
	go svc.ProcessPendingIdeas(context.Background())
	return &IdeaHandler{
		service: svc,
	}
}

func (h *IdeaHandler) ListIdeas(c *gin.Context) {
	ideas, err := h.service.ListIdeas(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al consultar el repositorio de ideas: %v", err)
		return
	}

	data := gin.H{
		"title": "Repositorio de ideas",
		"ideas": ideas,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_ideas_list.html", data)
		return
	}

	config.Render(c, http.StatusOK, "ideas_list.html", data)
}

func (h *IdeaHandler) NewIdea(c *gin.Context) {
	data := gin.H{
		"title": "Nueva idea",
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_ideas_new.html", data)
		return
	}

	config.Render(c, http.StatusOK, "ideas_new.html", data)
}

func (h *IdeaHandler) CreateIdea(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	content := strings.TrimSpace(c.PostForm("content"))

	fileHeader, fileErr := c.FormFile("doc")
	if fileErr == nil && fileHeader != nil && fileHeader.Size > 0 {
		extractedText, err := services.ExtractTextFromUpload(fileHeader)
		if err != nil {
			data := gin.H{
				"title":     "Nueva idea",
				"error":     err.Error(),
				"ideaTitle": title,
				"content":   content,
			}
			if c.GetHeader("HX-Request") == "true" {
				config.Render(c, http.StatusBadRequest, "_ideas_new.html", data)
				return
			}
			config.Render(c, http.StatusBadRequest, "ideas_new.html", data)
			return
		}

		if title == "" {
			ext := filepath.Ext(fileHeader.Filename)
			title = strings.TrimSpace(strings.TrimSuffix(fileHeader.Filename, ext))
		}

		if content == "" {
			content = extractedText
		} else if extractedText != "" {
			content = content + "\n\n" + extractedText
		}
	}

	if title == "" || content == "" {
		data := gin.H{
			"title":     "Nueva idea",
			"error":     "El título y el contenido son obligatorios (o sube un archivo .txt, .md, .pdf, .docx, .csv).",
			"ideaTitle": title,
			"content":   content,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_ideas_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "ideas_new.html", data)
		return
	}

	_, err := h.service.CreateIdeasFromInput(c.Request.Context(), c.GetString("user"), title, content)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al guardar la idea: %v", err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		ideas, err := h.service.ListIdeas(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, "La idea se guardó, pero no se pudo actualizar el listado: %v", err)
			return
		}

		config.Render(c, http.StatusOK, "_ideas_list.html", gin.H{
			"title": "Repositorio de ideas",
			"ideas": ideas,
		})
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/ideas/list")
}

func (h *IdeaHandler) NetworkView(c *gin.Context) {
	config.Render(c, http.StatusOK, "network.html", gin.H{
		"title": "Red de pensamiento",
	})
}

func (h *IdeaHandler) NetworkGraphJSON(c *gin.Context) {
	graph, err := h.service.GetNetworkGraph(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo cargar la red de conocimiento"})
		return
	}

	c.JSON(http.StatusOK, graph)
}

type AskNetworkRequest struct {
	Question string `json:"question" form:"question"`
}

type AskNetworkSource struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	HopDistance  int     `json:"hopDistance"`
	RelationType string  `json:"relationType"`
	LinkedFrom   string  `json:"linkedFrom"`
	Weight       float64 `json:"weight"`
}

type AskNetworkResponse struct {
	Question string             `json:"question"`
	Answer   string             `json:"answer"`
	Fallback bool               `json:"fallback"`
	Sources  []AskNetworkSource `json:"sources"`
}

func (h *IdeaHandler) AskNetworkGraph(c *gin.Context) {
	var req AskNetworkRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Consulta inválida"})
		return
	}

	cleanedQuestion := strings.TrimSpace(req.Question)
	if cleanedQuestion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Por favor escribe una pregunta para consultar tu red"})
		return
	}

	res, err := h.service.QueryGraph(c.Request.Context(), cleanedQuestion)
	if err != nil {
		opts := services.DefaultTraversalOptions()
		subGraph, _, subErr := h.service.SearchAndExpand(c.Request.Context(), cleanedQuestion, 3, opts)
		if subErr == nil && subGraph != nil && len(subGraph.Nodes) > 0 {
			titleMap := make(map[string]string)
			for _, n := range subGraph.Nodes {
				titleMap[n.ID.String()] = n.Title
			}
			sources := make([]AskNetworkSource, 0, len(subGraph.Nodes))
			for _, n := range subGraph.Nodes {
				parentTitle := titleMap[n.LinkedFromID.String()]
				if parentTitle == "" && n.HopDistance == 0 {
					parentTitle = "Idea principal"
				}
				sources = append(sources, AskNetworkSource{
					ID:           n.ID.String(),
					Title:        n.Title,
					HopDistance:  n.HopDistance,
					RelationType: n.RelationType,
					LinkedFrom:   parentTitle,
					Weight:       n.Weight,
				})
			}
			c.JSON(http.StatusOK, AskNetworkResponse{
				Question: cleanedQuestion,
				Answer:   "No pude sintetizar una respuesta en este momento, pero encontré estas ideas y conexiones en tu red que podrían interesarte:",
				Fallback: true,
				Sources:  sources,
			})
			return
		}

		c.JSON(http.StatusOK, AskNetworkResponse{
			Question: cleanedQuestion,
			Answer:   "No encontré ideas en tu red relacionadas con esta consulta. Intenta con otros términos o crea nuevas notas.",
			Fallback: true,
			Sources:  []AskNetworkSource{},
		})
		return
	}

	titleMap := make(map[string]string)
	if res.SubGraph != nil {
		for _, n := range res.SubGraph.Nodes {
			titleMap[n.ID.String()] = n.Title
		}
	}

	sources := make([]AskNetworkSource, 0)
	if res.SubGraph != nil {
		for _, n := range res.SubGraph.Nodes {
			parentTitle := titleMap[n.LinkedFromID.String()]
			if parentTitle == "" && n.HopDistance == 0 {
				parentTitle = "Idea principal"
			}
			sources = append(sources, AskNetworkSource{
				ID:           n.ID.String(),
				Title:        n.Title,
				HopDistance:  n.HopDistance,
				RelationType: n.RelationType,
				LinkedFrom:   parentTitle,
				Weight:       n.Weight,
			})
		}
	}

	answerText := res.Answer
	fallback := false
	if strings.TrimSpace(answerText) == "" {
		fallback = true
		if len(sources) > 0 {
			answerText = "No pude sintetizar una respuesta en este momento, pero encontré estas ideas y conexiones en tu red que podrían interesarte:"
		} else {
			answerText = "No encontré ideas en tu red relacionadas con esta consulta. Intenta con otros términos o crea nuevas notas."
		}
	}

	c.JSON(http.StatusOK, AskNetworkResponse{
		Question: cleanedQuestion,
		Answer:   answerText,
		Fallback: fallback,
		Sources:  sources,
	})
}

func (h *IdeaHandler) IdeaDetail(c *gin.Context) {
	ideaID := c.Param("id")
	detail, err := h.service.GetIdeaDetail(c.Request.Context(), ideaID)
	if err != nil {
		c.String(http.StatusNotFound, "No se encontró la idea solicitada: %v", err)
		return
	}

	data := gin.H{
		"title":  detail.Idea.Title,
		"detail": detail,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_idea_detail.html", data)
		return
	}

	config.Render(c, http.StatusOK, "idea_detail.html", data)
}

func (h *IdeaHandler) EditIdeaView(c *gin.Context) {
	ideaID := c.Param("id")
	detail, err := h.service.GetIdeaDetail(c.Request.Context(), ideaID)
	if err != nil {
		c.String(http.StatusNotFound, "No se encontró la idea solicitada: %v", err)
		return
	}

	data := gin.H{
		"title":     "Editar idea",
		"ideaID":    detail.Idea.ID,
		"ideaTitle": detail.Idea.Title,
		"content":   detail.Idea.Content,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_idea_edit.html", data)
		return
	}

	config.Render(c, http.StatusOK, "idea_edit.html", data)
}

func (h *IdeaHandler) UpdateIdea(c *gin.Context) {
	ideaID := c.Param("id")
	title := strings.TrimSpace(c.PostForm("title"))
	content := strings.TrimSpace(c.PostForm("content"))

	if title == "" || content == "" {
		data := gin.H{
			"title":     "Editar idea",
			"error":     "El título y el contenido son obligatorios.",
			"ideaID":    ideaID,
			"ideaTitle": title,
			"content":   content,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_idea_edit.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "idea_edit.html", data)
		return
	}

	detail, err := h.service.UpdateIdea(c.Request.Context(), ideaID, title, content)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al actualizar la idea: %v", err)
		return
	}

	data := gin.H{
		"title":  detail.Idea.Title,
		"detail": detail,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_idea_detail.html", data)
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/ideas/"+ideaID)
}

func (h *IdeaHandler) ReanalyzeIdea(c *gin.Context) {
	ideaID := c.Param("id")
	detail, err := h.service.ReanalyzeIdea(c.Request.Context(), ideaID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al volver a analizar la idea: %v", err)
		return
	}

	data := gin.H{
		"title":  detail.Idea.Title,
		"detail": detail,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_idea_detail.html", data)
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/ideas/"+ideaID)
}

func (h *IdeaHandler) DeleteIdeaFragment(c *gin.Context) {
	ideaID := c.Param("id")
	if err := h.service.DeleteIdeaFragment(c.Request.Context(), ideaID); err != nil {
		c.String(http.StatusInternalServerError, "Error al eliminar el fragmento: %v", err)
		return
	}

	h.renderIdeasListOrRedirect(c)
}

func (h *IdeaHandler) DeleteIdeaGroup(c *gin.Context) {
	ideaID := c.Param("id")
	if err := h.service.DeleteIdeaGroup(c.Request.Context(), ideaID); err != nil {
		c.String(http.StatusInternalServerError, "Error al eliminar la idea completa: %v", err)
		return
	}

	h.renderIdeasListOrRedirect(c)
}

func (h *IdeaHandler) renderIdeasListOrRedirect(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		ideas, err := h.service.ListIdeas(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, "Error al actualizar el repositorio de ideas: %v", err)
			return
		}

		config.Render(c, http.StatusOK, "_ideas_list.html", gin.H{
			"title": "Repositorio de ideas",
			"ideas": ideas,
		})
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/ideas/list")
}
