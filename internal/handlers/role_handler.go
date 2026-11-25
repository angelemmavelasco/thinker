package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/config"
	"thinker/internal/middleware"
	"thinker/internal/services"
	"thinker/internal/urls"
)

type RoleHandler struct {
	roleService *services.RoleService
}

func NewRoleHandler(roleService *services.RoleService) *RoleHandler {
	return &RoleHandler{roleService: roleService}
}

func (h *RoleHandler) ListRolesView(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	roles, err := h.roleService.ListRolesWithUsage(c.Request.Context(), actor)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		c.String(http.StatusInternalServerError, "Error al consultar roles")
		return
	}

	data := gin.H{
		"title": "Gestión de roles",
		"roles": roles,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_roles_list.html", data)
		return
	}

	config.Render(c, http.StatusOK, "roles_list.html", data)
}

func (h *RoleHandler) CreateRole(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	description := strings.TrimSpace(c.PostForm("description"))

	_, err := h.roleService.CreateRole(c.Request.Context(), actor, name, description)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		h.renderRolesWithError(c, actor, http.StatusBadRequest, err.Error(), name, description)
		return
	}

	h.renderRolesListOrRedirect(c, actor, "Rol creado correctamente.")
}

func (h *RoleHandler) UpdateRole(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.renderRolesWithError(c, actor, http.StatusBadRequest, "Identificador de rol inválido.", "", "")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	description := strings.TrimSpace(c.PostForm("description"))

	if _, err := h.roleService.UpdateRole(c.Request.Context(), actor, roleID, name, description); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		h.renderRolesWithError(c, actor, http.StatusBadRequest, err.Error(), "", "")
		return
	}

	h.renderRolesListOrRedirect(c, actor, "Rol actualizado correctamente.")
}

func (h *RoleHandler) DeleteRole(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	roleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.renderRolesWithError(c, actor, http.StatusBadRequest, "Identificador de rol inválido.", "", "")
		return
	}

	if err := h.roleService.DeleteRole(c.Request.Context(), actor, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.String(http.StatusForbidden, "Acceso denegado")
			return
		}
		h.renderRolesWithError(c, actor, http.StatusBadRequest, err.Error(), "", "")
		return
	}

	h.renderRolesListOrRedirect(c, actor, "Rol eliminado correctamente.")
}

func (h *RoleHandler) renderRolesWithError(c *gin.Context, actor services.Actor, status int, errMsg, name, description string) {
	roles, _ := h.roleService.ListRolesWithUsage(c.Request.Context(), actor)
	data := gin.H{
		"title":           "Gestión de roles",
		"roles":           roles,
		"error":           errMsg,
		"roleName":        name,
		"roleDescription": description,
	}
	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, status, "_roles_list.html", data)
		return
	}
	config.Render(c, status, "roles_list.html", data)
}

func (h *RoleHandler) renderRolesListOrRedirect(c *gin.Context, actor services.Actor, successMsg string) {
	if c.GetHeader("HX-Request") == "true" {
		roles, err := h.roleService.ListRolesWithUsage(c.Request.Context(), actor)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error al actualizar listado de roles")
			return
		}
		config.Render(c, http.StatusOK, "_roles_list.html", gin.H{
			"title":   "Gestión de roles",
			"roles":   roles,
			"success": successMsg,
		})
		return
	}
	c.Redirect(http.StatusSeeOther, urls.Get("roles:list"))
}
