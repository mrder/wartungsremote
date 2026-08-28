package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestChangeOwnPasswordRejectsShortPassword(t *testing.T) {
	s := &Service{Argon2: DefaultArgon2Params()}
	err := s.ChangeOwnPassword(context.Background(), uuid.Nil, "tooshort")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestHashAndVerifyPasswordRoundTrip(t *testing.T) {
	params := DefaultArgon2Params()
	hash, err := HashPassword("correct horse battery staple", params)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected correct password to verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	params := DefaultArgon2Params()
	hash, err := HashPassword("correct horse battery staple", params)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail verification")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword("", DefaultArgon2Params()); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("anything", "not-a-valid-hash"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}

func TestTwoHashesOfSamePasswordDiffer(t *testing.T) {
	params := DefaultArgon2Params()
	h1, err := HashPassword("same password", params)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, err := HashPassword("same password", params)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Fatal("expected distinct salts to produce distinct hashes")
	}
}
