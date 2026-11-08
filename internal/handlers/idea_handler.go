package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/services"
)

type IdeaHandler struct {
	queries *database.Queries
	service *services.IdeaService
}

func NewIdeaHandler(queries *database.Queries) *IdeaHandler {
	return &IdeaHandler{
		queries: queries,
		service: services.NewIdeaService(queries),
	}
}

func (h *IdeaHandler) ListIdeas(c *gin.Context) {
	ideas, err := h.queries.ListIdeas(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al consultar el repositorio de ideas: %v", err)
		return
	}

	//define context for templates
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

	if title == "" || content == "" {
		data := gin.H{
			"title":     "Nueva idea",
			"error":     "El título y el contenido son obligatorios.",
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

	if _, err := h.service.CreateIdea(c.Request.Context(), c.GetString("user"), title, content); err != nil {
		c.String(http.StatusInternalServerError, "Error al guardar la idea: %v", err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		ideas, err := h.queries.ListIdeas(c.Request.Context())
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
