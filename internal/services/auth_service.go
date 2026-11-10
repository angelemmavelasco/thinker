package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"thinker/internal/auth"
	"thinker/internal/database"
)

const (
	ActionTokenTypeEmailVerification = "email_verification"
	ActionTokenTypePasswordReset     = "password_reset"

	ActionTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 7 * 24 * time.Hour
	MaxLoginFails   = 5
	BaseLockoutTime = 15 * time.Minute
	MaxLockoutTime  = 24 * time.Hour
)

var (
	ErrInvalidCredentials     = errors.New("Credenciales inválidas")
	ErrEmailAlreadyRegistered = errors.New("no se pudo completar el registro con el correo proporcionado")
	ErrEmailNotVerified       = errors.New("Debes confirmar tu correo electrónico antes de iniciar sesión. Revisa tu bandeja de entrada.")
	ErrAccountLocked          = errors.New("La cuenta está bloqueada temporalmente por múltiples intentos fallidos. Intenta más tarde.")
	ErrInvalidOrExpiredToken  = errors.New("el enlace o token es inválido, ya fue utilizado o ha expirado")
	ErrRefreshTokenReused     = errors.New("se detectó reutilización de un token de sesión revocado; todas las sesiones han sido cerradas por seguridad")
	ErrSessionInvalidated     = errors.New("la sesión ya no es válida")
)

type SessionTokens struct {
	User             database.User
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type AuthService struct {
	queries *database.Queries
	jwtMgr  *auth.JWTManager
	mailer  *MailerService
	google  *GoogleOAuthService
}

func NewAuthService(
	queries *database.Queries,
	jwtMgr *auth.JWTManager,
	mailer *MailerService,
	google *GoogleOAuthService,
) *AuthService {
	auth.VerifyDummyPassword("warmup-init")
	return &AuthService{
		queries: queries,
		jwtMgr:  jwtMgr,
		mailer:  mailer,
		google:  google,
	}
}

func (s *AuthService) JWTManager() *auth.JWTManager {
	return s.jwtMgr
}

func (s *AuthService) GoogleOAuth() *GoogleOAuthService {
	return s.google
}

func (s *AuthService) Register(ctx context.Context, rawEmail, password string) (database.User, string, error) {
	email, err := auth.NormalizeEmail(rawEmail)
	if err != nil {
		return database.User{}, "", err
	}
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return database.User{}, "", err
	}

	if existing, err := s.queries.GetUserByEmail(ctx, email); err == nil && existing.ID != uuid.Nil {
		auth.VerifyDummyPassword(password)
		return database.User{}, "", ErrEmailAlreadyRegistered
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return database.User{}, "", fmt.Errorf("error procesando credenciales")
	}

	user, err := s.queries.CreateUser(ctx, database.CreateUserParams{
		ID:            uuid.New(),
		Email:         email,
		PasswordHash:  passwordHash,
		Role:          "member",
		AuthProvider:  "local",
		GoogleID:      "",
		EmailVerified: 0,
	})
	if err != nil {
		return database.User{}, "", ErrEmailAlreadyRegistered
	}

	rawVerifyToken, err := s.issueActionToken(ctx, user.ID, ActionTokenTypeEmailVerification)
	if err != nil {
		return user, "", err
	}

	if s.mailer != nil {
		_ = s.mailer.SendVerificationEmail(user.Email, rawVerifyToken)
	}

	return user, rawVerifyToken, nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, rawToken string) (database.User, error) {
	if rawToken == "" {
		return database.User{}, ErrInvalidOrExpiredToken
	}
	tokenHash := auth.HashToken(rawToken)

	actToken, err := s.queries.GetActionTokenByHash(ctx, database.GetActionTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: ActionTokenTypeEmailVerification,
	})
	if err != nil {
		return database.User{}, ErrInvalidOrExpiredToken
	}

	now := time.Now().UTC()
	if actToken.ConsumedAt.Valid || now.After(actToken.ExpiresAt.UTC()) {
		return database.User{}, ErrInvalidOrExpiredToken
	}

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return database.User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	if _, err := qtx.ConsumeActionToken(ctx, database.ConsumeActionTokenParams{
		ID:         actToken.ID,
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return database.User{}, ErrInvalidOrExpiredToken
	}

	user, err := qtx.MarkUserEmailVerified(ctx, actToken.UserID)
	if err != nil {
		return database.User{}, ErrInvalidOrExpiredToken
	}

	if err := tx.Commit(); err != nil {
		return database.User{}, err
	}
	return user, nil
}

func (s *AuthService) ResendVerification(ctx context.Context, rawEmail string) (string, error) {
	email, err := auth.NormalizeEmail(rawEmail)
	if err != nil {
		return "", nil
	}
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil || user.EmailVerified != 0 {
		return "", nil
	}

	rawToken, err := s.issueActionToken(ctx, user.ID, ActionTokenTypeEmailVerification)
	if err != nil {
		return "", nil
	}
	if s.mailer != nil {
		_ = s.mailer.SendVerificationEmail(user.Email, rawToken)
	}
	return rawToken, nil
}

func (s *AuthService) Login(ctx context.Context, rawEmail, password, ipAddress, userAgent string) (*SessionTokens, error) {
	email, err := auth.NormalizeEmail(rawEmail)
	if err != nil || password == "" {
		auth.VerifyDummyPassword(password)
		return nil, ErrInvalidCredentials
	}

	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		auth.VerifyDummyPassword(password)
		return nil, ErrInvalidCredentials
	}

	now := time.Now().UTC()
	if user.LockedUntil.Valid && now.Before(user.LockedUntil.Time.UTC()) {
		auth.VerifyDummyPassword(password)
		return nil, ErrAccountLocked
	}

	matched, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil || !matched {
		s.recordFailedLogin(ctx, user, now)
		return nil, ErrInvalidCredentials
	}

	_ = s.queries.ResetFailedLoginAttempts(ctx, user.ID)

	if user.EmailVerified == 0 {
		return nil, ErrEmailNotVerified
	}

	if auth.NeedsPasswordRehash(user.PasswordHash) {
		if newHash, hashErr := auth.HashPassword(password); hashErr == nil {
			if updatedUser, updateErr := s.queries.UpdateUserPasswordAndBumpTokenVersion(ctx, database.UpdateUserPasswordAndBumpTokenVersionParams{
				ID:           user.ID,
				PasswordHash: newHash,
			}); updateErr == nil {
				user = updatedUser
			}
		}
	}

	return s.createInitialSession(ctx, user, ipAddress, userAgent)
}

func (s *AuthService) recordFailedLogin(ctx context.Context, user database.User, now time.Time) {
	attempts := user.FailedLoginAttempts + 1
	var lockedUntil sql.NullTime

	if attempts >= MaxLoginFails {
		shift := int(attempts - MaxLoginFails)
		if shift > 6 {
			shift = 6
		}
		lockDuration := BaseLockoutTime * time.Duration(1<<shift)
		if lockDuration > MaxLockoutTime {
			lockDuration = MaxLockoutTime
		}
		lockedUntil = sql.NullTime{
			Time:  now.Add(lockDuration),
			Valid: true,
		}
	}

	_, _ = s.queries.RecordFailedLoginAttempt(ctx, database.RecordFailedLoginAttemptParams{
		ID:                  user.ID,
		FailedLoginAttempts: attempts,
		LockedUntil:         lockedUntil,
	})
}

func (s *AuthService) AuthenticateGoogleUser(ctx context.Context, profile *GoogleProfile, ipAddress, userAgent string) (*SessionTokens, error) {
	if profile == nil || !profile.EmailVerified {
		return nil, ErrGoogleEmailUnverified
	}

	email, err := auth.NormalizeEmail(profile.Email)
	if err != nil {
		return nil, err
	}

	var user database.User
	userByGoogle, err := s.queries.GetUserByGoogleID(ctx, profile.Sub)
	if err == nil {
		user = userByGoogle
	} else {
		userByEmail, emailErr := s.queries.GetUserByEmail(ctx, email)
		if emailErr == nil {
			linkedUser, linkErr := s.queries.LinkGoogleAccount(ctx, database.LinkGoogleAccountParams{
				ID:       userByEmail.ID,
				GoogleID: profile.Sub,
			})
			if linkErr != nil {
				return nil, linkErr
			}
			user = linkedUser
		} else {
			createdUser, createErr := s.queries.CreateUser(ctx, database.CreateUserParams{
				ID:            uuid.New(),
				Email:         email,
				PasswordHash:  "",
				Role:          "member",
				AuthProvider:  "google",
				GoogleID:      profile.Sub,
				EmailVerified: 1,
			})
			if createErr != nil {
				return nil, createErr
			}
			user = createdUser
		}
	}

	return s.createInitialSession(ctx, user, ipAddress, userAgent)
}

func (s *AuthService) createInitialSession(ctx context.Context, user database.User, ipAddress, userAgent string) (*SessionTokens, error) {
	accessToken, accessExp, err := s.jwtMgr.GenerateAccessToken(user.ID, user.Email, user.Role, user.TokenVersion)
	if err != nil {
		return nil, err
	}

	rawRefresh, refreshHash, err := auth.GenerateOpaqueToken(auth.DefaultTokenBytes)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	refreshExp := now.Add(RefreshTokenTTL)
	familyID := uuid.New()

	_, err = s.queries.CreateRefreshToken(ctx, database.CreateRefreshTokenParams{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  familyID,
		UserAgent: userAgent,
		IpAddress: ipAddress,
		ExpiresAt: refreshExp,
	})
	if err != nil {
		return nil, err
	}

	return &SessionTokens{
		User:             user,
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExp,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func (s *AuthService) RefreshSession(ctx context.Context, rawRefreshToken, ipAddress, userAgent string) (*SessionTokens, error) {
	if rawRefreshToken == "" {
		return nil, ErrSessionInvalidated
	}

	tokenHash := auth.HashToken(rawRefreshToken)
	storedToken, err := s.queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrSessionInvalidated
	}

	now := time.Now().UTC()

	if storedToken.Revoked != 0 {
		_ = s.queries.RevokeRefreshTokenFamily(ctx, database.RevokeRefreshTokenFamilyParams{
			FamilyID:  storedToken.FamilyID,
			RevokedAt: sql.NullTime{Time: now, Valid: true},
		})
		_, _ = s.queries.IncrementUserTokenVersion(ctx, storedToken.UserID)
		return nil, ErrRefreshTokenReused
	}

	if now.After(storedToken.ExpiresAt.UTC()) {
		_ = s.queries.RevokeRefreshTokenFamily(ctx, database.RevokeRefreshTokenFamilyParams{
			FamilyID:  storedToken.FamilyID,
			RevokedAt: sql.NullTime{Time: now, Valid: true},
		})
		return nil, ErrSessionInvalidated
	}

	user, err := s.queries.GetUserByID(ctx, storedToken.UserID)
	if err != nil || user.EmailVerified == 0 {
		return nil, ErrSessionInvalidated
	}

	newRawRefresh, newRefreshHash, err := auth.GenerateOpaqueToken(auth.DefaultTokenBytes)
	if err != nil {
		return nil, err
	}
	newTokenID := uuid.New()

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if err := qtx.MarkRefreshTokenRotated(ctx, database.MarkRefreshTokenRotatedParams{
		ID:           storedToken.ID,
		ReplacedByID: uuid.NullUUID{UUID: newTokenID, Valid: true},
		RevokedAt:    sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return nil, err
	}

	if _, err := qtx.CreateRefreshToken(ctx, database.CreateRefreshTokenParams{
		ID:        newTokenID,
		UserID:    user.ID,
		TokenHash: newRefreshHash,
		FamilyID:  storedToken.FamilyID,
		UserAgent: userAgent,
		IpAddress: ipAddress,
		ExpiresAt: storedToken.ExpiresAt.UTC(),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	accessToken, accessExp, err := s.jwtMgr.GenerateAccessToken(user.ID, user.Email, user.Role, user.TokenVersion)
	if err != nil {
		return nil, err
	}

	return &SessionTokens{
		User:             user,
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExp,
		RefreshToken:     newRawRefresh,
		RefreshExpiresAt: storedToken.ExpiresAt.UTC(),
	}, nil
}

func (s *AuthService) ValidateUserTokenVersion(ctx context.Context, userID uuid.UUID, claimVersion int32) (database.User, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return database.User{}, ErrSessionInvalidated
	}
	if user.TokenVersion != claimVersion || user.EmailVerified == 0 {
		return database.User{}, ErrSessionInvalidated
	}
	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	tokenHash := auth.HashToken(rawRefreshToken)
	storedToken, err := s.queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil
	}
	return s.queries.RevokeRefreshTokenFamily(ctx, database.RevokeRefreshTokenFamilyParams{
		FamilyID:  storedToken.FamilyID,
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	})
}

func (s *AuthService) LogoutAllSessions(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	if _, err := qtx.IncrementUserTokenVersion(ctx, userID); err != nil {
		return err
	}
	if err := qtx.RevokeAllUserRefreshTokens(ctx, database.RevokeAllUserRefreshTokensParams{
		UserID:    userID,
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, rawEmail string) (string, error) {
	email, err := auth.NormalizeEmail(rawEmail)
	if err != nil {
		auth.VerifyDummyPassword(rawEmail)
		return "", nil
	}

	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		auth.VerifyDummyPassword(email)
		return "", nil
	}

	rawToken, err := s.issueActionToken(ctx, user.ID, ActionTokenTypePasswordReset)
	if err != nil {
		return "", nil
	}

	if s.mailer != nil {
		_ = s.mailer.SendPasswordResetEmail(user.Email, rawToken)
	}
	return rawToken, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if rawToken == "" {
		return ErrInvalidOrExpiredToken
	}
	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	tokenHash := auth.HashToken(rawToken)
	actToken, err := s.queries.GetActionTokenByHash(ctx, database.GetActionTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: ActionTokenTypePasswordReset,
	})
	if err != nil {
		return ErrInvalidOrExpiredToken
	}

	now := time.Now().UTC()
	if actToken.ConsumedAt.Valid || now.After(actToken.ExpiresAt.UTC()) {
		return ErrInvalidOrExpiredToken
	}

	newPasswordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.queries.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if _, err := qtx.ConsumeActionToken(ctx, database.ConsumeActionTokenParams{
		ID:         actToken.ID,
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return ErrInvalidOrExpiredToken
	}

	if _, err := qtx.UpdateUserPasswordAndBumpTokenVersion(ctx, database.UpdateUserPasswordAndBumpTokenVersionParams{
		ID:           actToken.UserID,
		PasswordHash: newPasswordHash,
	}); err != nil {
		return err
	}

	if _, err := qtx.MarkUserEmailVerified(ctx, actToken.UserID); err != nil {
		return err
	}

	if err := qtx.RevokeAllUserRefreshTokens(ctx, database.RevokeAllUserRefreshTokensParams{
		UserID:    actToken.UserID,
		RevokedAt: sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *AuthService) issueActionToken(ctx context.Context, userID uuid.UUID, tokenType string) (string, error) {
	now := time.Now().UTC()
	_ = s.queries.InvalidateUserActionTokensByType(ctx, database.InvalidateUserActionTokensByTypeParams{
		UserID:     userID,
		TokenType:  tokenType,
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
	})

	rawToken, tokenHash, err := auth.GenerateOpaqueToken(auth.DefaultTokenBytes)
	if err != nil {
		return "", err
	}

	_, err = s.queries.CreateActionToken(ctx, database.CreateActionTokenParams{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		TokenType: tokenType,
		ExpiresAt: now.Add(ActionTokenTTL),
	})
	if err != nil {
		return "", err
	}
	return rawToken, nil
}
