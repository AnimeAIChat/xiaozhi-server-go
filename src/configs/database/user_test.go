package database

import (
	"strings"
	"testing"
)

func TestGenerateInitialAdminPassword(t *testing.T) {
	password, err := generateInitialAdminPassword()
	if err != nil {
		t.Fatalf("generateInitialAdminPassword() error = %v", err)
	}
	if len(password) != initialAdminPasswordLength {
		t.Fatalf("password length = %d, want %d", len(password), initialAdminPasswordLength)
	}
	for _, character := range password {
		if !strings.ContainsRune(initialAdminPasswordAlphabet, character) {
			t.Fatalf("password contains character outside the allowed alphabet: %q", character)
		}
	}
}
