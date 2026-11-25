package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/auth"
	"thinker/internal/database"
	"thinker/internal/services"
)

func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'",
		)

		if c.Request.Method == http.MethodTrace {
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}

		c.Next()
	}
}

func setAuthenticatedContext(c *gin.Context, authService *services.AuthService, user database.User) bool {
	roles, err := authService.ListUserRoles(c.Request.Context(), user.ID)
	if err != nil {
		return false
	}
	actor := services.NewActorFromUser(user, roles)
	c.Set("actor", actor)
	c.Set("user", user.ID.String())
	c.Set("user_email", user.Email)
	c.Set("user_role", user.Role)
	c.Set("is_superuser", actor.IsSuperuser)
	c.Set("user_roles", actor.Roles)
	displayName, initial := auth.UserDisplayName("", "", user.Email)
	c.Set("user_name", displayName)
	c.Set("user_initial", initial)
	return true
}

func ActorFromContext(c *gin.Context) (services.Actor, bool) {
	raw, exists := c.Get("actor")
	if !exists {
		return services.Actor{}, false
	}
	actor, ok := raw.(services.Actor)
	if !ok || !actor.IsValid() {
		return services.Actor{}, false
	}
	return actor, true
}

func RequireAuthMiddleware(authService *services.AuthService, secureCookies bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "no-store")

		accessToken := ExtractAccessToken(c, secureCookies)
		if accessToken != "" {
			claims, err := authService.JWTManager().ValidateAccessToken(accessToken)
			if err == nil {
				userID, parseErr := uuid.Parse(claims.Subject)
				if parseErr == nil {
					user, verErr := authService.ValidateUserTokenVersion(c.Request.Context(), userID, claims.TokenVersion)
					if verErr == nil {
						if !setAuthenticatedContext(c, authService, user) {
							c.AbortWithStatus(http.StatusInternalServerError)
							return
						}
						c.Next()
						return
					}
				}
			}
		}

		rawRefresh := ExtractRefreshToken(c, secureCookies)
		if rawRefresh != "" {
			newTokens, err := authService.RefreshSession(
				c.Request.Context(),
				rawRefresh,
				c.ClientIP(),
				c.Request.UserAgent(),
			)
			if err == nil && newTokens != nil {
				SetSessionCookies(c, newTokens, secureCookies)
				if !setAuthenticatedContext(c, authService, newTokens.User) {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
				c.Next()
				return
			}
		}

		ClearSessionCookies(c, secureCookies)
		abortUnauthenticated(c)
	}
}

func RequireSuperuserMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := ActorFromContext(c)
		if !ok {
			abortUnauthenticated(c)
			return
		}
		if !actor.IsSuperuser {
			abortForbidden(c)
			return
		}
		c.Next()
	}
}

func abortForbidden(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "No tienes permisos para acceder a este recurso"})
		return
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: requiere privilegios de superusuario"})
}

func abortUnauthenticated(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.Contains(c.GetHeader("Accept"), "application/json") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No autenticado"})
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/auth/login")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Redirect(http.StatusSeeOther, "/auth/login")
	c.Abort()
}
