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

func (h *UserHandler) ListUsersView(c *gin.Context) {
	users, err := h.queries.ListUsers(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al consultar el listado de usuarios asociados: %v", err)
		return
	}

	c.HTML(http.StatusOK, "users_list.html", gin.H{
		"title": "Usuarios asociados",
		"users": users,
	})
}
