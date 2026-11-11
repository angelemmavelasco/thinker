package auth

import (
	"strings"
	"unicode"
)

// UserDisplayName returns the formatted full name (first name + first last name)
// and the avatar initial letter.
// If firstName and lastName are provided, it uses them.
// Otherwise, it parses the local part of the email.
func UserDisplayName(firstName, lastName, email string) (string, string) {
	first := strings.TrimSpace(firstName)
	last := strings.TrimSpace(lastName)

	if first != "" {
		name := first
		if last != "" {
			name += " " + last
		}
		runes := []rune(first)
		initial := strings.ToUpper(string(runes[0]))
		return name, initial
	}

	parts := strings.Split(email, "@")
	if len(parts) == 0 || parts[0] == "" {
		return "Usuario", "U"
	}

	local := parts[0]
	tokens := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})

	cleanWord := func(s string) string {
		s = strings.TrimFunc(s, func(r rune) bool {
			return unicode.IsDigit(r)
		})
		if len(s) == 0 {
			return ""
		}
		runes := []rune(s)
		return strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:]))
	}

	var words []string
	for _, t := range tokens {
		cw := cleanWord(t)
		if cw != "" {
			words = append(words, cw)
		}
	}

	if len(words) == 0 {
		return "Usuario", "U"
	}
	if len(words) == 1 {
		return words[0], string([]rune(words[0])[0])
	}

	// Primer nombre y primer apellido
	firstNameRes := words[0]
	lastNameRes := words[len(words)-1]
	fullName := firstNameRes + " " + lastNameRes
	initial := string([]rune(firstNameRes)[0])
	return fullName, initial
}
