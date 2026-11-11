package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/auth"
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
						c.Set("user", user.ID.String())
						c.Set("user_email", user.Email)
						c.Set("user_role", user.Role)
						displayName, initial := auth.UserDisplayName("", "", user.Email)
						c.Set("user_name", displayName)
						c.Set("user_initial", initial)
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
				c.Set("user", newTokens.User.ID.String())
				c.Set("user_email", newTokens.User.Email)
				c.Set("user_role", newTokens.User.Role)
				displayName, initial := auth.UserDisplayName("", "", newTokens.User.Email)
				c.Set("user_name", displayName)
				c.Set("user_initial", initial)
				c.Next()
				return
			}
		}

		ClearSessionCookies(c, secureCookies)
		abortUnauthenticated(c)
	}
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
