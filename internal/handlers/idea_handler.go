package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
)

type IdeaHandler struct {
	queries *database.Queries
}

func NewIdeaHandler(queries *database.Queries) *IdeaHandler {
	return &IdeaHandler{queries: queries}
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

// func (h *IdeaHandler) CreateIdea(c *gin.Context) error {
// 	return nil
// }
