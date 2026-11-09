package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// tokenBytes is the amount of entropy in session and invite tokens.
const tokenBytes = 32

// GenerateToken returns a cryptographically random URL-safe token.
func GenerateToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the value stored in the database for a session token. The
// raw session token only lives in the client's cookie, so a database leak does
// not expose live sessions.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
