package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/services"
)

type UserHandler struct {
	service *services.UserService
}

func NewUserHandler(queries *database.Queries) *UserHandler {
	return &UserHandler{
		service: services.NewUserService(queries),
	}
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.service.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudieron obtener los usuarios"})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) ListUsersView(c *gin.Context) {
	users, err := h.service.ListUsers(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al consultar el listado de usuarios asociados: %v", err)
		return
	}

	data := gin.H{
		"title": "Usuarios asociados",
		"users": users,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_users_list.html", data)
		return
	}

	config.Render(c, http.StatusOK, "users_list.html", data)
}

func (h *UserHandler) NewUser(c *gin.Context) {
	data := gin.H{
		"title": "Nuevo usuario",
		"role":  "member",
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_users_new.html", data)
		return
	}

	config.Render(c, http.StatusOK, "users_new.html", data)
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := strings.TrimSpace(c.PostForm("password"))
	role := strings.TrimSpace(c.PostForm("role"))

	if email == "" || password == "" {
		data := gin.H{
			"title": "Nuevo usuario",
			"error": "El correo electrónico y la contraseña son obligatorios.",
			"email": email,
			"role":  role,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_users_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "users_new.html", data)
		return
	}

	if _, err := h.service.CreateUser(c.Request.Context(), email, password, role); err != nil {
		data := gin.H{
			"title": "Nuevo usuario",
			"error": "No se pudo registrar el usuario. Verifica que el correo sea válido, único y la contraseña tenga al menos 8 caracteres.",
			"email": email,
			"role":  role,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_users_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "users_new.html", data)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		users, err := h.service.ListUsers(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, "El usuario se guardó, pero no se pudo actualizar el listado: %v", err)
			return
		}

		config.Render(c, http.StatusOK, "_users_list.html", gin.H{
			"title": "Usuarios asociados",
			"users": users,
		})
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/users")
}
