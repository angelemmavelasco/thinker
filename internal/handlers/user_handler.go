package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/middleware"
	"thinker/internal/services"
	"thinker/internal/urls"
)

type UserHandler struct {
	service     *services.UserService
	roleService *services.RoleService
}

func NewUserHandler(queries *database.Queries, roleService *services.RoleService) *UserHandler {
	return &UserHandler{
		service:     services.NewUserService(queries),
		roleService: roleService,
	}
}

func parseRoleIDList(raw []string) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		if parsed, err := uuid.Parse(strings.TrimSpace(s)); err == nil && parsed != uuid.Nil {
			result = append(result, parsed)
		}
	}
	return result
}

func parseFormBool(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No autenticado"})
		return
	}

	users, err := h.service.ListUsers(c.Request.Context(), actor)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudieron obtener los usuarios"})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) ListUsersView(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	users, err := h.service.ListUsers(c.Request.Context(), actor)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		c.String(http.StatusInternalServerError, "Error al consultar el listado de usuarios asociados")
		return
	}

	allRoles, _ := h.roleService.ListRoles(c.Request.Context())

	data := gin.H{
		"title":        "Usuarios asociados",
		"users":        users,
		"allRoles":     allRoles,
		"currentActor": actor.UserID.String(),
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_users_list.html", data)
		return
	}

	config.Render(c, http.StatusOK, "users_list.html", data)
}

func (h *UserHandler) NewUser(c *gin.Context) {
	allRoles, _ := h.roleService.ListRoles(c.Request.Context())
	data := gin.H{
		"title":    "Nuevo usuario",
		"allRoles": allRoles,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_users_new.html", data)
		return
	}

	config.Render(c, http.StatusOK, "users_new.html", data)
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	email := strings.TrimSpace(c.PostForm("email"))
	password := strings.TrimSpace(c.PostForm("password"))
	isSuperuser := parseFormBool(c.PostForm("is_superuser"))
	roleIDs := parseRoleIDList(c.PostFormArray("role_ids"))
	allRoles, _ := h.roleService.ListRoles(c.Request.Context())

	if email == "" || password == "" {
		data := gin.H{
			"title":       "Nuevo usuario",
			"error":       "El correo electrónico y la contraseña son obligatorios.",
			"email":       email,
			"isSuperuser": isSuperuser,
			"allRoles":    allRoles,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_users_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "users_new.html", data)
		return
	}

	if _, err := h.service.CreateUser(c.Request.Context(), actor, email, password, isSuperuser, roleIDs); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		data := gin.H{
			"title":       "Nuevo usuario",
			"error":       "No se pudo registrar el usuario. Verifica que el correo sea válido, único y la contraseña tenga al menos 8 caracteres.",
			"email":       email,
			"isSuperuser": isSuperuser,
			"allRoles":    allRoles,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_users_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "users_new.html", data)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		users, err := h.service.ListUsers(c.Request.Context(), actor)
		if err != nil {
			c.String(http.StatusInternalServerError, "El usuario se guardó, pero no se pudo actualizar el listado")
			return
		}

		config.Render(c, http.StatusOK, "_users_list.html", gin.H{
			"title":        "Usuarios asociados",
			"users":        users,
			"allRoles":     allRoles,
			"currentActor": actor.UserID.String(),
			"success":      "Usuario creado correctamente.",
		})
		return
	}

	c.Redirect(http.StatusSeeOther, urls.Get("users:list"))
}

func (h *UserHandler) UpdateUserAccess(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Identificador de usuario inválido")
		return
	}

	isSuperuser := parseFormBool(c.PostForm("is_superuser"))
	roleIDs := parseRoleIDList(c.PostFormArray("role_ids"))

	_, updateErr := h.service.UpdateUserAccess(c.Request.Context(), actor, targetID, isSuperuser, roleIDs)
	if errors.Is(updateErr, services.ErrForbidden) {
		c.String(http.StatusForbidden, "Acceso denegado")
		return
	}

	users, _ := h.service.ListUsers(c.Request.Context(), actor)
	allRoles, _ := h.roleService.ListRoles(c.Request.Context())

	data := gin.H{
		"title":        "Usuarios asociados",
		"users":        users,
		"allRoles":     allRoles,
		"currentActor": actor.UserID.String(),
	}

	status := http.StatusOK
	if updateErr != nil {
		status = http.StatusBadRequest
		data["error"] = updateErr.Error()
	} else {
		data["success"] = "Permisos de usuario actualizados."
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, status, "_users_list.html", data)
		return
	}

	if updateErr != nil {
		config.Render(c, status, "users_list.html", data)
		return
	}

	c.Redirect(http.StatusSeeOther, urls.Get("users:list"))
}
