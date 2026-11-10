package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"

	"thinker/internal/auth"
	"thinker/internal/config"
	"thinker/internal/database"
)

func setupTestAuthService(t *testing.T) (*AuthService, *database.Queries, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:auth_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed opening in-memory sqlite: %v", err)
	}

	ctx := context.Background()
	if err := database.InitSchema(ctx, conn); err != nil {
		_ = conn.Close()
		t.Fatalf("InitSchema failed: %v", err)
	}

	secret := []byte("super-secret-cryptographic-key-32b!")
	jwtMgr, err := auth.NewJWTManager(secret, 15*time.Minute)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("NewJWTManager failed: %v", err)
	}

	cfg := &config.BaseConfig{
		Debug:      false,
		AppBaseURL: "http://127.0.0.1:8080",
		JWTSecret:  secret,
	}
	queries := database.New(conn)
	mailer := NewMailerService(cfg)
	googleSvc := NewGoogleOAuthService(cfg)
	authSvc := NewAuthService(queries, jwtMgr, mailer, googleSvc)

	cleanup := func() {
		_ = conn.Close()
	}
	return authSvc, queries, cleanup
}

func TestAuthLifecycle_Verification_Rotation_Lockout_And_Reset(t *testing.T) {
	authSvc, _, cleanup := setupTestAuthService(t)
	defer cleanup()
	ctx := context.Background()

	user, verifyToken, err := authSvc.Register(ctx, "  Ada.Lovelace@Example.COM ", "StrongPassphrase!1843")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.Email != "ada.lovelace@example.com" {
		t.Fatalf("expected normalized email 'ada.lovelace@example.com', got %q", user.Email)
	}

	if _, _, err := authSvc.Register(ctx, "ADA.LOVELACE@example.com", "AnotherPassphrase!123"); !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("expected ErrEmailAlreadyRegistered on duplicate email, got: %v", err)
	}

	if _, err := authSvc.Login(ctx, "ada.lovelace@example.com", "StrongPassphrase!1843", "127.0.0.1", "GoTest"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("expected ErrEmailNotVerified before confirming email, got: %v", err)
	}

	if _, err := authSvc.VerifyEmail(ctx, verifyToken); err != nil {
		t.Fatalf("VerifyEmail failed: %v", err)
	}
	if _, err := authSvc.VerifyEmail(ctx, verifyToken); !errors.Is(err, ErrInvalidOrExpiredToken) {
		t.Fatalf("expected single-use verification token to fail on second use, got: %v", err)
	}

	_, errMissingUser := authSvc.Login(ctx, "ghost@example.com", "SomePassword123!", "127.0.0.1", "GoTest")
	_, errWrongPass := authSvc.Login(ctx, "ada.lovelace@example.com", "WrongPassword123!", "127.0.0.1", "GoTest")
	if !errors.Is(errMissingUser, ErrInvalidCredentials) || !errors.Is(errWrongPass, ErrInvalidCredentials) {
		t.Fatalf("expected both errors to be ErrInvalidCredentials, got %v and %v", errMissingUser, errWrongPass)
	}
	if errMissingUser.Error() != "Credenciales inválidas" || errWrongPass.Error() != "Credenciales inválidas" {
		t.Fatalf("expected exact message 'Credenciales inválidas'")
	}

	for i := 0; i < 4; i++ {
		_, _ = authSvc.Login(ctx, "ada.lovelace@example.com", "WrongPassword123!", "127.0.0.1", "GoTest")
	}
	if _, err := authSvc.Login(ctx, "ada.lovelace@example.com", "StrongPassphrase!1843", "127.0.0.1", "GoTest"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked after 5 failed attempts, got: %v", err)
	}

	resetToken, err := authSvc.RequestPasswordReset(ctx, "ADA.LOVELACE@EXAMPLE.COM")
	if err != nil || resetToken == "" {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}
	if err := authSvc.ResetPassword(ctx, resetToken, "NewUltraSecurePass!2026"); err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	if err := authSvc.ResetPassword(ctx, resetToken, "AnotherPass!2026"); !errors.Is(err, ErrInvalidOrExpiredToken) {
		t.Fatalf("expected consumed reset token to be rejected, got: %v", err)
	}

	session1, err := authSvc.Login(ctx, "ada.lovelace@example.com", "NewUltraSecurePass!2026", "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}

	session2, err := authSvc.RefreshSession(ctx, session1.RefreshToken, "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("RefreshSession failed: %v", err)
	}
	if session2.RefreshToken == session1.RefreshToken {
		t.Fatalf("expected rotated refresh token to be distinct from old token")
	}

	if _, err := authSvc.RefreshSession(ctx, session1.RefreshToken, "127.0.0.1", "AttackerUA"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("expected ErrRefreshTokenReused when replaying rotated token, got: %v", err)
	}
	if _, err := authSvc.RefreshSession(ctx, session2.RefreshToken, "127.0.0.1", "GoTest"); err == nil {
		t.Fatalf("expected family revocation to invalidate session2 refresh token")
	}

	session3, err := authSvc.Login(ctx, "ada.lovelace@example.com", "NewUltraSecurePass!2026", "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("Login for session3 failed: %v", err)
	}
	claims3, err := authSvc.JWTManager().ValidateAccessToken(session3.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if _, err := authSvc.ValidateUserTokenVersion(ctx, session3.User.ID, claims3.TokenVersion); err != nil {
		t.Fatalf("expected token_version to be valid before LogoutAllSessions: %v", err)
	}

	if err := authSvc.LogoutAllSessions(ctx, session3.User.ID); err != nil {
		t.Fatalf("LogoutAllSessions failed: %v", err)
	}
	if _, err := authSvc.ValidateUserTokenVersion(ctx, session3.User.ID, claims3.TokenVersion); !errors.Is(err, ErrSessionInvalidated) {
		t.Fatalf("expected JWT with old token_version to be rejected after LogoutAllSessions, got: %v", err)
	}
}

func TestBcryptPasswordUpgradeOnLogin(t *testing.T) {
	authSvc, queries, cleanup := setupTestAuthService(t)
	defer cleanup()
	ctx := context.Background()

	rawPassword := "OldBcryptSecret123!"
	bcryptHashBytes, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword failed: %v", err)
	}
	bcryptHash := string(bcryptHashBytes)

	userID := uuid.New()
	email := "grace.hopper@example.com"
	_, err = queries.CreateUser(ctx, database.CreateUserParams{
		ID:            userID,
		Email:         email,
		PasswordHash:  bcryptHash,
		Role:          "member",
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 1,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	session, err := authSvc.Login(ctx, email, rawPassword, "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("Login with bcrypt hash failed: %v", err)
	}
	if session == nil || session.AccessToken == "" {
		t.Fatalf("expected valid session after logging in with bcrypt hash")
	}

	updatedUser, err := queries.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !strings.HasPrefix(updatedUser.PasswordHash, "$argon2id$") {
		t.Fatalf("expected password hash to be upgraded to Argon2id, got: %s", updatedUser.PasswordHash)
	}

	session2, err := authSvc.Login(ctx, email, rawPassword, "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("Second login using newly upgraded Argon2id hash failed: %v", err)
	}
	if session2 == nil || session2.AccessToken == "" {
		t.Fatalf("expected valid session on second login")
	}
}
