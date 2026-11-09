package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ExpectedJWTAlg    = "HS256"
	ExpectedJWTTyp    = "JWT"
	ExpectedJWTIssuer = "thinker"
	MinSecretKeyBytes = 32
	DefaultAccessTTL  = 15 * time.Minute
)

var (
	ErrInsufficientKeyEntropy = errors.New("el secreto JWT debe tener al menos 256 bits (32 bytes) de entropía")
	ErrMalformedToken         = errors.New("formato de token inválido")
	ErrUnexpectedAlgorithm    = errors.New("algoritmo de firma JWT no permitido")
	ErrInvalidSignature       = errors.New("firma de token inválida")
	ErrExpiredToken           = errors.New("el token ha expirado")
	ErrInvalidClaims          = errors.New("claims del token inválidos")
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type AccessClaims struct {
	Subject      string `json:"sub"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	TokenVersion int32  `json:"ver"`
	Issuer       string `json:"iss"`
	JWTID        string `json:"jti"`
	IssuedAt     int64  `json:"iat"`
	NotBefore    int64  `json:"nbf"`
	ExpiresAt    int64  `json:"exp"`
}

type JWTManager struct {
	secret    []byte
	accessTTL time.Duration
}

func NewJWTManager(secret []byte, accessTTL time.Duration) (*JWTManager, error) {
	if len(secret) < MinSecretKeyBytes {
		return nil, ErrInsufficientKeyEntropy
	}
	if accessTTL <= 0 || accessTTL > 30*time.Minute {
		accessTTL = DefaultAccessTTL
	}
	keyCopy := make([]byte, len(secret))
	copy(keyCopy, secret)
	return &JWTManager{
		secret:    keyCopy,
		accessTTL: accessTTL,
	}, nil
}

func (m *JWTManager) GenerateAccessToken(userID uuid.UUID, email, role string, tokenVersion int32) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.accessTTL)

	header := jwtHeader{
		Alg: ExpectedJWTAlg,
		Typ: ExpectedJWTTyp,
	}
	claims := AccessClaims{
		Subject:      userID.String(),
		Email:        email,
		Role:         role,
		TokenVersion: tokenVersion,
		Issuer:       ExpectedJWTIssuer,
		JWTID:        uuid.NewString(),
		IssuedAt:     now.Unix(),
		NotBefore:    now.Unix(),
		ExpiresAt:    expiresAt.Unix(),
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, err
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := encodedHeader + "." + encodedClaims

	sig := m.computeHMAC([]byte(signingInput))
	encodedSig := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + encodedSig, expiresAt, nil
}

func (m *JWTManager) ValidateAccessToken(tokenString string) (*AccessClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrMalformedToken
	}

	if header.Alg != ExpectedJWTAlg || header.Typ != ExpectedJWTTyp {
		return nil, fmt.Errorf("%w: recibido alg=%q typ=%q", ErrUnexpectedAlgorithm, header.Alg, header.Typ)
	}

	providedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrMalformedToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := m.computeHMAC([]byte(signingInput))

	if !hmac.Equal(providedSig, expectedSig) {
		return nil, ErrInvalidSignature
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var claims AccessClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, ErrMalformedToken
	}

	nowUnix := time.Now().UTC().Unix()
	if claims.ExpiresAt <= nowUnix {
		return nil, ErrExpiredToken
	}
	if claims.NotBefore > nowUnix+30 {
		return nil, ErrInvalidClaims
	}
	if claims.Issuer != ExpectedJWTIssuer || claims.Subject == "" || claims.TokenVersion < 1 {
		return nil, ErrInvalidClaims
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, ErrInvalidClaims
	}

	return &claims, nil
}

func (m *JWTManager) computeHMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(data)
	return mac.Sum(nil)
}
