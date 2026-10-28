package auth

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	manager := NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	token, expiresAt, err := manager.Issue(42)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("expiresAt must be in the future")
	}
	userID, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if userID != 42 {
		t.Fatalf("Parse() userID = %d, want 42", userID)
	}
}

func TestTokenRejectsWrongSecret(t *testing.T) {
	issuer := NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	token, _, err := issuer.Issue(7)
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokenManager("abcdef0123456789abcdef0123456789", "test", time.Hour)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("expected token validation to fail with a different secret")
	}
}
