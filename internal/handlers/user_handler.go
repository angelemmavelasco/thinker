package handlers

import (
	"net/http"
	"thinker/internal/database"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	queries *database.Queries
}

func NewUserHandler(queries *database.Queries) *UserHandler {
	return &UserHandler{queries: queries}
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.queries.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudieron obtener los usuarios"})
		return
	}

	c.JSON(http.StatusOK, users)
}
