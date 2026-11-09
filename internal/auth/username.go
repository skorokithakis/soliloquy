// Package auth contains password hashing, token generation, username rules
// and the in-memory login throttle.
package auth

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidUsername is returned when a username does not follow the rules in
// the PRD: 3 to 20 characters from a-z, 0-9, _ and -.
var ErrInvalidUsername = errors.New("username must be 3 to 20 characters using a-z, 0-9, _ and -")

var usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{3,20}$`)

// NormalizeUsername trims and lowercases a username. Usernames are stored in
// their canonical lowercase form because they are case-insensitive.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidateUsername normalizes a username and checks it against the rules. It
// returns the canonical lowercase form.
func ValidateUsername(username string) (string, error) {
	username = NormalizeUsername(username)
	if !usernamePattern.MatchString(username) {
		return "", ErrInvalidUsername
	}
	return username, nil
}
