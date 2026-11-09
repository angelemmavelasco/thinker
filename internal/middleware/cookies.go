package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"thinker/internal/services"
)

const (
	hostPrefixAccessCookie  = "__Host-thinker_access"
	hostPrefixRefreshCookie = "__Host-thinker_refresh"
	hostPrefixCSRFCookie    = "__Host-thinker_csrf"
	hostPrefixOAuthCookie   = "__Host-thinker_oauth"

	devAccessCookie  = "thinker_access"
	devRefreshCookie = "thinker_refresh"
	devCSRFCookie    = "thinker_csrf"
	devOAuthCookie   = "thinker_oauth"
)

func AccessCookieName(secure bool) string {
	if secure {
		return hostPrefixAccessCookie
	}
	return devAccessCookie
}

func RefreshCookieName(secure bool) string {
	if secure {
		return hostPrefixRefreshCookie
	}
	return devRefreshCookie
}

func CSRFCookieName(secure bool) string {
	if secure {
		return hostPrefixCSRFCookie
	}
	return devCSRFCookie
}

func OAuthCookieName(secure bool) string {
	if secure {
		return hostPrefixOAuthCookie
	}
	return devOAuthCookie
}

func SetSessionCookies(c *gin.Context, tokens *services.SessionTokens, secure bool) {
	now := time.Now().UTC()
	accessMaxAge := int(tokens.AccessExpiresAt.Sub(now).Seconds())
	if accessMaxAge < 60 {
		accessMaxAge = 15 * 60
	}
	refreshMaxAge := int(tokens.RefreshExpiresAt.Sub(now).Seconds())
	if refreshMaxAge < 60 {
		refreshMaxAge = int(services.RefreshTokenTTL.Seconds())
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessCookieName(secure),
		Value:    tokens.AccessToken,
		Path:     "/",
		MaxAge:   accessMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     RefreshCookieName(secure),
		Value:    tokens.RefreshToken,
		Path:     "/",
		MaxAge:   refreshMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookies(c *gin.Context, secure bool) {
	for _, name := range []string{
		AccessCookieName(secure),
		RefreshCookieName(secure),
		hostPrefixAccessCookie,
		hostPrefixRefreshCookie,
		devAccessCookie,
		devRefreshCookie,
	} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func ExtractAccessToken(c *gin.Context, secure bool) string {
	if val, err := c.Cookie(AccessCookieName(secure)); err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if val, err := c.Cookie(devAccessCookie); err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

func ExtractRefreshToken(c *gin.Context, secure bool) string {
	if val, err := c.Cookie(RefreshCookieName(secure)); err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if val, err := c.Cookie(devRefreshCookie); err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return ""
}
