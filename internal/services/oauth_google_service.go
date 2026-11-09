package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"thinker/internal/config"
)

const (
	defaultGoogleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultGoogleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	defaultGoogleUserInfoEndpoint = "https://www.googleapis.com/oauth2/v3/userinfo"
)

var (
	ErrGoogleOAuthNotConfigured = errors.New("Google OAuth no está configurado en el servidor")
	ErrGoogleOAuthExchange      = errors.New("no se pudo completar la autenticación con Google")
	ErrGoogleEmailUnverified    = errors.New("la cuenta de Google no tiene un correo electrónico verificado")
)

type GoogleProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type GoogleOAuthService struct {
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	AuthEndpoint     string
	TokenEndpoint    string
	UserInfoEndpoint string
	HTTPClient       *http.Client
}

func NewGoogleOAuthService(cfg *config.BaseConfig) *GoogleOAuthService {
	return &GoogleOAuthService{
		ClientID:         cfg.GoogleClientID,
		ClientSecret:     cfg.GoogleClientSecret,
		RedirectURL:      cfg.GoogleRedirectURL,
		AuthEndpoint:     defaultGoogleAuthEndpoint,
		TokenEndpoint:    defaultGoogleTokenEndpoint,
		UserInfoEndpoint: defaultGoogleUserInfoEndpoint,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (g *GoogleOAuthService) IsConfigured() bool {
	return strings.TrimSpace(g.ClientID) != "" && strings.TrimSpace(g.ClientSecret) != ""
}

func (g *GoogleOAuthService) BuildAuthorizationURL(state, codeChallenge string) (string, error) {
	if !g.IsConfigured() {
		return "", ErrGoogleOAuthNotConfigured
	}

	u, err := url.Parse(g.AuthEndpoint)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("client_id", g.ClientID)
	q.Set("redirect_uri", g.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (g *GoogleOAuthService) ExchangeCodeForProfile(ctx context.Context, code, codeVerifier string) (*GoogleProfile, error) {
	if !g.IsConfigured() {
		return nil, ErrGoogleOAuthNotConfigured
	}

	form := url.Values{}
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.ClientSecret)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", g.RedirectURL)

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, ErrGoogleOAuthExchange
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := g.HTTPClient.Do(tokenReq)
	if err != nil {
		return nil, ErrGoogleOAuthExchange
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, tokenResp.Body)
		return nil, ErrGoogleOAuthExchange
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(tokenResp.Body, 64*1024)).Decode(&tokenPayload); err != nil || tokenPayload.AccessToken == "" {
		return nil, ErrGoogleOAuthExchange
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, g.UserInfoEndpoint, nil)
	if err != nil {
		return nil, ErrGoogleOAuthExchange
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := g.HTTPClient.Do(userReq)
	if err != nil {
		return nil, ErrGoogleOAuthExchange
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, userResp.Body)
		return nil, ErrGoogleOAuthExchange
	}

	var profile GoogleProfile
	if err := json.NewDecoder(io.LimitReader(userResp.Body, 64*1024)).Decode(&profile); err != nil {
		return nil, fmt.Errorf("%w: respuesta de perfil inválida", ErrGoogleOAuthExchange)
	}

	if profile.Sub == "" || profile.Email == "" {
		return nil, ErrGoogleOAuthExchange
	}
	if !profile.EmailVerified {
		return nil, ErrGoogleEmailUnverified
	}

	return &profile, nil
}
