package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	t.Run("valid token", func(t *testing.T) {
		token, err := MakeJWT(userID, secret, time.Hour)
		if err != nil {
			t.Fatalf("MakeJWT() error = %v", err)
		}

		gotID, err := ValidateJWT(token, secret)
		if err != nil {
			t.Fatalf("ValidateJWT() error = %v", err)
		}

		if gotID != userID {
			t.Errorf("got UUID %v, want %v", gotID, userID)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token, err := MakeJWT(userID, secret, -time.Hour)
		if err != nil {
			t.Fatalf("MakeJWT() error = %v", err)
		}

		_, err = ValidateJWT(token, secret)
		if err == nil {
			t.Fatal("expected an error for expired token, got nil")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		token, err := MakeJWT(userID, secret, time.Hour)
		if err != nil {
			t.Fatalf("MakeJWT() error = %v", err)
		}

		_, err = ValidateJWT(token, "wrong-secret")
		if err == nil {
			t.Fatal("expected an error for wrong secret, got nil")
		}
	})
}

func TestGetBearerToken(t *testing.T) {
	t.Run("valid bearer token", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Authorization", "Bearer abc123")

		got, err := GetBearerToken(headers)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "abc123" {
			t.Errorf("got %q, want %q", got, "abc123")
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		headers := http.Header{}

		_, err := GetBearerToken(headers)
		if err == nil {
			t.Fatal("expected error for missing authorization header")
		}
	})

	t.Run("invalid authorization format", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Authorization", "Basic abc123")

		_, err := GetBearerToken(headers)
		if err == nil {
			t.Fatal("expected error for invalid authorization format")
		}
	})
}
