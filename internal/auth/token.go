package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// tokenBytes is the amount of entropy in session and password reset tokens.
const tokenBytes = 32

// GenerateToken returns a cryptographically random URL-safe token.
func GenerateToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

const (
	inviteTokenLength   = 8
	inviteTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// GenerateInviteToken returns a short invite token made of alphanumeric
// characters. Bytes that would introduce modulo bias are rejected.
//
// Short is deliberate, so that links are easy to share. 62^8 is about 2.2e14,
// and an invite is single use and expires after seven days, so online guessing
// is not practical. Sessions and password resets can take over an existing
// account, so they keep using GenerateToken.
func GenerateInviteToken() (string, error) {
	const limit = 256 - 256%len(inviteTokenAlphabet)

	buf := make([]byte, 1)
	token := make([]byte, inviteTokenLength)
	for i := range token {
		for {
			if _, err := rand.Read(buf); err != nil {
				return "", err
			}
			if int(buf[0]) < limit {
				token[i] = inviteTokenAlphabet[int(buf[0])%len(inviteTokenAlphabet)]
				break
			}
		}
	}
	return string(token), nil
}

// HashToken returns the value stored in the database for a session token. The
// raw session token only lives in the client's cookie, so a database leak does
// not expose live sessions.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
