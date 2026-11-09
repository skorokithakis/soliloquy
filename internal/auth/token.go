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

// ShortIDLength is the length of the short, alphanumeric IDs used for invites
// and for the public IDs of threads and comments.
const ShortIDLength = 8

const shortIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GenerateShortID returns a short ID made of alphanumeric characters. Bytes
// that would introduce modulo bias are rejected.
//
// Short is deliberate, so that links are easy to share. 62^8 is about 2.2e14.
// Invites are single use and expire after seven days, so online guessing is not
// practical. Thread and comment public IDs only need to be unguessable enough
// that they cannot be enumerated. Sessions and password resets can take over an
// existing account, so they keep using GenerateToken.
func GenerateShortID() (string, error) {
	const limit = 256 - 256%len(shortIDAlphabet)

	buf := make([]byte, 1)
	token := make([]byte, ShortIDLength)
	for i := range token {
		for {
			if _, err := rand.Read(buf); err != nil {
				return "", err
			}
			if int(buf[0]) < limit {
				token[i] = shortIDAlphabet[int(buf[0])%len(shortIDAlphabet)]
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
