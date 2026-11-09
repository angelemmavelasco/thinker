package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const DefaultTokenBytes = 32

func GenerateOpaqueToken(byteLen int) (rawToken string, tokenHash string, err error) {
	if byteLen < DefaultTokenBytes {
		byteLen = DefaultTokenBytes
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("error leyendo bytes aleatorios del sistema operativo: %w", err)
	}
	rawToken = base64.RawURLEncoding.EncodeToString(buf)
	tokenHash = HashToken(rawToken)
	return rawToken, tokenHash, nil
}

func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func GeneratePKCE() (verifier string, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("error generando PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
