package auth

import (
	"strings"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Error("CheckPassword rejected the correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword accepted the wrong password")
	}
}

func TestHashPasswordRejectsEmptyAndOversizedInput(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Error("HashPassword accepted an empty password")
	}
	if _, err := HashPassword(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Error("HashPassword accepted a password longer than bcrypt's limit")
	}
}
