package auth

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrEmptyEmail   = errors.New("el correo electrónico es obligatorio")
	ErrInvalidEmail = errors.New("el formato del correo electrónico no es válido")
)

func NormalizeEmail(raw string) (string, error) {
	normalized := norm.NFC.String(raw)
	normalized = strings.TrimSpace(normalized)
	normalized = strings.ToLower(normalized)

	if normalized == "" {
		return "", ErrEmptyEmail
	}
	if len(normalized) > 254 {
		return "", ErrInvalidEmail
	}

	for _, r := range normalized {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", ErrInvalidEmail
		}
	}

	parsed, err := mail.ParseAddress(normalized)
	if err != nil {
		return "", ErrInvalidEmail
	}
	if parsed.Name != "" || strings.ToLower(parsed.Address) != normalized {
		return "", ErrInvalidEmail
	}

	parts := strings.Split(normalized, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrInvalidEmail
	}
	domain := parts[1]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", ErrInvalidEmail
	}

	return normalized, nil
}
