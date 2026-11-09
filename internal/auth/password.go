package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2Params = &Argon2Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

var (
	ErrInvalidHashFormat         = errors.New("formato de hash Argon2id inválido")
	ErrIncompatibleArgon2Version = errors.New("versión de Argon2 incompatible")
	ErrPasswordTooShort          = errors.New("la contraseña debe tener al menos 8 caracteres")
	ErrPasswordTooLong           = errors.New("la contraseña no puede exceder 128 caracteres")

	dummyHashOnce sync.Once
	dummyHashPHC  string
)

func ValidatePasswordStrength(password string) error {
	runeCount := utf8.RuneCountInString(password)
	if runeCount < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 || runeCount > 128 {
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultArgon2Params)
}

func HashPasswordWithParams(password string, p *Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("error generando sal criptográfica: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.Memory,
		p.Iterations,
		p.Parallelism,
		b64Salt,
		b64Hash,
	)
	return encoded, nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	if encodedHash == "" || !strings.HasPrefix(encodedHash, "$argon2id$") {
		VerifyDummyPassword(password)
		return false, ErrInvalidHashFormat
	}

	params, salt, expectedHash, err := decodeArgon2Hash(encodedHash)
	if err != nil {
		VerifyDummyPassword(password)
		return false, err
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	if subtle.ConstantTimeCompare(computedHash, expectedHash) == 1 {
		return true, nil
	}
	return false, nil
}

func VerifyDummyPassword(password string) {
	dummyHashOnce.Do(func() {
		h, err := HashPasswordWithParams("dummy-baseline-secret-password-do-not-use", DefaultArgon2Params)
		if err != nil {
			dummyHashPHC = "$argon2id$v=19$m=65536,t=3,p=4$c29tZXJhbmRvbXNhbHQxMjM0$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			return
		}
		dummyHashPHC = h
	})

	params, salt, expectedHash, err := decodeArgon2Hash(dummyHashPHC)
	if err != nil {
		return
	}
	computed := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)
	_ = subtle.ConstantTimeCompare(computed, expectedHash)
}

func decodeArgon2Hash(encodedHash string) (*Argon2Params, []byte, []byte, error) {
	vals := strings.Split(encodedHash, "$")
	if len(vals) != 6 || vals[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(vals[2], "v=%d", &version); err != nil {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleArgon2Version
	}

	p := &Argon2Params{}
	if _, err := fmt.Sscanf(vals[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	if p.Memory == 0 || p.Iterations == 0 || p.Parallelism == 0 {
		return nil, nil, nil, ErrInvalidHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(vals[4])
	if err != nil || len(salt) == 0 {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	p.SaltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(vals[5])
	if err != nil || len(hash) == 0 {
		return nil, nil, nil, ErrInvalidHashFormat
	}
	p.KeyLength = uint32(len(hash))

	return p, salt, hash, nil
}
