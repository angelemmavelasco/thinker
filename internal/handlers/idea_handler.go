package handlers

import (
	"net/http"
	"thinker/internal/database"

	"github.com/gin-gonic/gin"
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

	c.HTML(http.StatusOK, "ideas_list.html", gin.H{
		"title": "Repositorio de ideas",
		"ideas": ideas,
	})

}
