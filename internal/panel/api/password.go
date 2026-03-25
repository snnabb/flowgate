package api

import (
	"errors"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const passwordPolicyMessage = "密码至少需要6个字符，不能包含空格"

func normalizePassword(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func preparePassword(value string) (string, error) {
	normalized := normalizePassword(value)
	if len(normalized) < 6 {
		return "", errors.New(passwordPolicyMessage)
	}
	return normalized, nil
}

func comparePassword(hash string, value string) error {
	rawErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(value))
	if rawErr == nil {
		return nil
	}

	normalized := normalizePassword(value)
	if normalized != value {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(normalized)); err == nil {
			return nil
		}
	}

	return rawErr
}
