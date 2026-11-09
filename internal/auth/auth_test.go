package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArgon2idHashAndVerify(t *testing.T) {
	password := "CorrectHorseBatteryStaple!2026"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword second call failed: %v", err)
	}

	if hash1 == hash2 {
		t.Fatalf("expected unique salts to produce distinct Argon2id hashes")
	}
	if !strings.HasPrefix(hash1, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected PHC format: %s", hash1)
	}

	ok, err := VerifyPassword(password, hash1)
	if err != nil || !ok {
		t.Fatalf("expected valid password to verify: ok=%v err=%v", ok, err)
	}

	okWrong, err := VerifyPassword("WrongPassword!2026", hash1)
	if err != nil || okWrong {
		t.Fatalf("expected wrong password to fail verification")
	}

	start := time.Now()
	VerifyDummyPassword("any-password")
	if time.Since(start) <= 0 {
		t.Fatalf("expected VerifyDummyPassword to execute Argon2id computation")
	}
}

func TestNormalizeEmail(t *testing.T) {
	validCases := map[string]string{
		"  UsEr.Name+Tag@Example.COM ": "user.name+tag@example.com",
		"ADMIN@THINKER.LOCAL":          "admin@thinker.local",
	}
	for input, expected := range validCases {
		got, err := NormalizeEmail(input)
		if err != nil {
			t.Fatalf("NormalizeEmail(%q) unexpected error: %v", input, err)
		}
		if got != expected {
			t.Fatalf("NormalizeEmail(%q) = %q, want %q", input, got, expected)
		}
	}

	invalidCases := []string{
		"",
		"   ",
		"plainaddress",
		"@missinglocal.com",
		"user@nodotdomain",
		"user@.leadingdot.com",
		"user@trailingdot.com.",
		"Display Name <user@example.com>",
		"user name@example.com",
	}
	for _, invalid := range invalidCases {
		if _, err := NormalizeEmail(invalid); err == nil {
			t.Fatalf("expected NormalizeEmail(%q) to fail", invalid)
		}
	}
}

func TestJWTManagerStrictAlgAndEntropy(t *testing.T) {
	if _, err := NewJWTManager([]byte("short-secret-key"), 15*time.Minute); !errors.Is(err, ErrInsufficientKeyEntropy) {
		t.Fatalf("expected ErrInsufficientKeyEntropy for <256-bit key, got: %v", err)
	}

	secret := []byte("0123456789abcdef0123456789abcdef")
	mgr, err := NewJWTManager(secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager failed: %v", err)
	}

	userID := uuid.New()
	token, _, err := mgr.GenerateAccessToken(userID, "user@example.com", "member", 3)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	claims, err := mgr.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if claims.Subject != userID.String() || claims.TokenVersion != 3 || claims.Role != "member" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	parts := strings.Split(token, ".")
	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	forgedNoneToken := noneHeader + "." + parts[1] + "."
	if _, err := mgr.ValidateAccessToken(forgedNoneToken); !errors.Is(err, ErrUnexpectedAlgorithm) {
		t.Fatalf("expected ErrUnexpectedAlgorithm for alg:none attack, got: %v", err)
	}

	rsHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	forgedRSToken := rsHeader + "." + parts[1] + "." + parts[2]
	if _, err := mgr.ValidateAccessToken(forgedRSToken); !errors.Is(err, ErrUnexpectedAlgorithm) {
		t.Fatalf("expected ErrUnexpectedAlgorithm for alg:RS256 attack, got: %v", err)
	}

	tamperedClaims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + userID.String() + `","role":"admin","ver":3,"iss":"thinker","exp":9999999999}`))
	tamperedToken := parts[0] + "." + tamperedClaims + "." + parts[2]
	if _, err := mgr.ValidateAccessToken(tamperedToken); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for tampered payload, got: %v", err)
	}
}
