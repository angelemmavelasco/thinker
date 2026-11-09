package handlers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thinker/internal/auth"
	"thinker/internal/config"
	"thinker/internal/middleware"
	"thinker/internal/services"
)

type AuthHandler struct {
	authService   *services.AuthService
	secureCookies bool
}

func NewAuthHandler(authService *services.AuthService, secureCookies bool) *AuthHandler {
	return &AuthHandler{
		authService:   authService,
		secureCookies: secureCookies,
	}
}

func (h *AuthHandler) baseAuthData(title string) gin.H {
	googleEnabled := h.authService.GoogleOAuth() != nil && h.authService.GoogleOAuth().IsConfigured()
	return gin.H{
		"title":         title,
		"googleEnabled": googleEnabled,
	}
}

func (h *AuthHandler) LoginView(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	data := h.baseAuthData("Iniciar sesión")
	config.Render(c, http.StatusOK, "login.html", data)
}

func (h *AuthHandler) Login(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")

	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	tokens, err := h.authService.Login(
		c.Request.Context(),
		email,
		password,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		data := h.baseAuthData("Iniciar sesión")
		data["email"] = email

		status := http.StatusUnauthorized
		if errors.Is(err, services.ErrEmailNotVerified) {
			status = http.StatusForbidden
			data["error"] = services.ErrEmailNotVerified.Error()
			data["unverifiedEmail"] = email
		} else if errors.Is(err, services.ErrAccountLocked) {
			status = http.StatusTooManyRequests
			data["error"] = services.ErrAccountLocked.Error()
		} else {
			data["error"] = services.ErrInvalidCredentials.Error()
		}

		config.Render(c, status, "login.html", data)
		return
	}

	middleware.SetSessionCookies(c, tokens, h.secureCookies)
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/web/ideas/list")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/web/ideas/list")
}

func (h *AuthHandler) RegisterView(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	data := h.baseAuthData("Crear cuenta")
	config.Render(c, http.StatusOK, "register.html", data)
}

func (h *AuthHandler) Register(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")

	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	passwordConfirm := c.PostForm("password_confirm")

	data := h.baseAuthData("Crear cuenta")
	data["email"] = email

	if password != passwordConfirm {
		data["error"] = "Las contraseñas no coinciden."
		config.Render(c, http.StatusBadRequest, "register.html", data)
		return
	}

	_, _, err := h.authService.Register(c.Request.Context(), email, password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidEmail) || errors.Is(err, auth.ErrEmptyEmail) ||
			errors.Is(err, auth.ErrPasswordTooShort) || errors.Is(err, auth.ErrPasswordTooLong) {
			data["error"] = err.Error()
		} else {
			data["error"] = "No se pudo registrar la cuenta. Verifica los datos o intenta iniciar sesión."
		}
		config.Render(c, http.StatusBadRequest, "register.html", data)
		return
	}

	loginData := h.baseAuthData("Iniciar sesión")
	loginData["email"] = email
	loginData["success"] = "Cuenta registrada. Te enviamos un enlace de confirmación (válido por 15 minutos). Debes confirmar tu correo antes de iniciar sesión."
	config.Render(c, http.StatusCreated, "login.html", loginData)
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	rawToken := strings.TrimSpace(c.Query("token"))

	loginData := h.baseAuthData("Iniciar sesión")
	user, err := h.authService.VerifyEmail(c.Request.Context(), rawToken)
	if err != nil {
		loginData["error"] = "El enlace de confirmación es inválido, ya fue utilizado o ha expirado (15 minutos)."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}

	loginData["email"] = user.Email
	loginData["success"] = "¡Correo electrónico confirmado con éxito! Ya puedes iniciar sesión."
	config.Render(c, http.StatusOK, "login.html", loginData)
}

func (h *AuthHandler) ResendVerification(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	email := strings.TrimSpace(c.PostForm("email"))

	_, _ = h.authService.ResendVerification(c.Request.Context(), email)

	loginData := h.baseAuthData("Iniciar sesión")
	loginData["email"] = email
	loginData["success"] = "Si la cuenta existe y está pendiente de activación, hemos enviado un nuevo enlace válido por 15 minutos."
	config.Render(c, http.StatusOK, "login.html", loginData)
}

func (h *AuthHandler) ForgotPasswordView(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	data := h.baseAuthData("Recuperar contraseña")
	config.Render(c, http.StatusOK, "forgot_password.html", data)
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	email := strings.TrimSpace(c.PostForm("email"))

	_, _ = h.authService.RequestPasswordReset(c.Request.Context(), email)

	data := h.baseAuthData("Recuperar contraseña")
	data["success"] = "Si el correo electrónico está registrado, recibirás un enlace de recuperación de un solo uso válido por 15 minutos."
	config.Render(c, http.StatusOK, "forgot_password.html", data)
}

func (h *AuthHandler) ResetPasswordView(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	rawToken := strings.TrimSpace(c.Query("token"))
	if rawToken == "" {
		loginData := h.baseAuthData("Iniciar sesión")
		loginData["error"] = "Enlace de recuperación inválido."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}

	data := h.baseAuthData("Restablecer contraseña")
	data["token"] = rawToken
	config.Render(c, http.StatusOK, "reset_password.html", data)
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	rawToken := strings.TrimSpace(c.PostForm("token"))
	password := c.PostForm("password")
	passwordConfirm := c.PostForm("password_confirm")

	data := h.baseAuthData("Restablecer contraseña")
	data["token"] = rawToken

	if password != passwordConfirm {
		data["error"] = "Las contraseñas no coinciden."
		config.Render(c, http.StatusBadRequest, "reset_password.html", data)
		return
	}

	if err := h.authService.ResetPassword(c.Request.Context(), rawToken, password); err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) || errors.Is(err, auth.ErrPasswordTooLong) {
			data["error"] = err.Error()
			config.Render(c, http.StatusBadRequest, "reset_password.html", data)
			return
		}
		loginData := h.baseAuthData("Iniciar sesión")
		loginData["error"] = "El enlace de recuperación es inválido, ya fue utilizado o ha expirado (15 minutos). Solicita uno nuevo."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}

	middleware.ClearSessionCookies(c, h.secureCookies)
	loginData := h.baseAuthData("Iniciar sesión")
	loginData["success"] = "Contraseña actualizada correctamente. Por seguridad, todas tus sesiones activas fueron cerradas."
	config.Render(c, http.StatusOK, "login.html", loginData)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	rawRefresh := middleware.ExtractRefreshToken(c, h.secureCookies)
	if rawRefresh == "" {
		middleware.ClearSessionCookies(c, h.secureCookies)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Sesión no válida"})
		return
	}

	newTokens, err := h.authService.RefreshSession(
		c.Request.Context(),
		rawRefresh,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		middleware.ClearSessionCookies(c, h.secureCookies)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Sesión expirada o revocada"})
		return
	}

	middleware.SetSessionCookies(c, newTokens, h.secureCookies)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "expires_at": newTokens.AccessExpiresAt.Unix()})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	rawRefresh := middleware.ExtractRefreshToken(c, h.secureCookies)
	_ = h.authService.Logout(c.Request.Context(), rawRefresh)

	middleware.ClearSessionCookies(c, h.secureCookies)
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/auth/login")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/auth/login")
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")

	userIDStr := c.GetString("user")
	if userIDStr == "" {
		accessToken := middleware.ExtractAccessToken(c, h.secureCookies)
		if accessToken != "" {
			if claims, err := h.authService.JWTManager().ValidateAccessToken(accessToken); err == nil {
				userIDStr = claims.Subject
			}
		}
	}

	if parsedID, err := uuid.Parse(userIDStr); err == nil {
		_ = h.authService.LogoutAllSessions(c.Request.Context(), parsedID)
	}

	middleware.ClearSessionCookies(c, h.secureCookies)
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/auth/login")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/auth/login")
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	googleSvc := h.authService.GoogleOAuth()
	if googleSvc == nil || !googleSvc.IsConfigured() {
		loginData := h.baseAuthData("Iniciar sesión")
		loginData["error"] = "El inicio de sesión con Google no está configurado en este entorno."
		config.Render(c, http.StatusServiceUnavailable, "login.html", loginData)
		return
	}

	state, _, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	verifier, challenge, err := auth.GeneratePKCE()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.OAuthCookieName(h.secureCookies),
		Value:    state + "|" + verifier,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	authURL, err := googleSvc.BuildAuthorizationURL(state, challenge)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	cookieName := middleware.OAuthCookieName(h.secureCookies)
	rawCookie, err := c.Cookie(cookieName)

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	loginData := h.baseAuthData("Iniciar sesión")
	if err != nil || rawCookie == "" {
		loginData["error"] = "La sesión de autorización con Google expiró o es inválida."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}

	parts := strings.SplitN(rawCookie, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		loginData["error"] = "Estado de autorización OAuth inválido."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}
	expectedState, codeVerifier := parts[0], parts[1]
	presentedState := strings.TrimSpace(c.Query("state"))
	code := strings.TrimSpace(c.Query("code"))

	if presentedState == "" || code == "" || subtle.ConstantTimeCompare([]byte(expectedState), []byte(presentedState)) != 1 {
		loginData["error"] = "Validación de seguridad OAuth fallida."
		config.Render(c, http.StatusBadRequest, "login.html", loginData)
		return
	}

	profile, err := h.authService.GoogleOAuth().ExchangeCodeForProfile(c.Request.Context(), code, codeVerifier)
	if err != nil {
		loginData["error"] = "No se pudo verificar tu cuenta con Google."
		config.Render(c, http.StatusUnauthorized, "login.html", loginData)
		return
	}

	tokens, err := h.authService.AuthenticateGoogleUser(
		c.Request.Context(),
		profile,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		loginData["error"] = "No se pudo iniciar sesión con la cuenta de Google proporcionada."
		config.Render(c, http.StatusUnauthorized, "login.html", loginData)
		return
	}

	middleware.SetSessionCookies(c, tokens, h.secureCookies)
	c.Redirect(http.StatusSeeOther, "/web/ideas/list")
}
