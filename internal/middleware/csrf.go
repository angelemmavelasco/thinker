package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"thinker/internal/auth"
)

func CSRFMiddleware(secureCookies bool) gin.HandlerFunc {
	cookieName := CSRFCookieName(secureCookies)

	return func(c *gin.Context) {
		csrfToken, err := c.Cookie(cookieName)
		if err != nil || len(csrfToken) < 32 {
			raw, _, genErr := auth.GenerateOpaqueToken(32)
			if genErr != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			csrfToken = raw
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     cookieName,
				Value:    csrfToken,
				Path:     "/",
				MaxAge:   12 * 3600,
				HttpOnly: true,
				Secure:   secureCookies,
				SameSite: http.SameSiteLaxMode,
			})
		}

		c.Set("csrf_token", csrfToken)

		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			c.Next()
			return
		}

		presented := strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
		if presented == "" {
			presented = strings.TrimSpace(c.PostForm("csrf_token"))
		}

		if presented == "" || subtle.ConstantTimeCompare([]byte(csrfToken), []byte(presented)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Token de seguridad CSRF inválido o ausente. Recarga la página e inténtalo de nuevo.",
			})
			return
		}

		c.Next()
	}
}
