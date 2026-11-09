package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Session is a logged-in browser. Only the hash of the session token is
// stored, so reading the database does not reveal live session cookies.
type Session struct {
	TokenHash string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// CreateSession stores a new session.
func (db *DB) CreateSession(ctx context.Context, userID int64, tokenHash string, now, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		tokenHash, userID, now.Unix(), expiresAt.Unix(),
	)
	return err
}

// CreateSessionIfPassword stores a new session only when the user's current
// password hash still matches expectedHash. It reports whether a session was
// inserted. This closes the login race with a concurrent password change.
func (db *DB) CreateSessionIfPassword(ctx context.Context, userID int64, expectedHash, tokenHash string, now, expiresAt time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		SELECT ?, id, ?, ? FROM users WHERE id = ? AND password_hash = ?`,
		tokenHash, now.Unix(), expiresAt.Unix(), userID, expectedHash,
	)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// SessionUser returns the session and user for a token hash. Expired sessions
// and sessions belonging to disabled or deleted users are removed and treated
// as absent.
func (db *DB) SessionUser(ctx context.Context, tokenHash string, now time.Time) (*Session, *User, error) {
	var (
		session   Session
		createdAt int64
		expiresAt int64
	)
	err := db.QueryRowContext(ctx, `
		SELECT token_hash, user_id, created_at, expires_at
		FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&session.TokenHash, &session.UserID, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	session.CreatedAt = timeFromUnix(createdAt)
	session.ExpiresAt = timeFromUnix(expiresAt)

	if !now.Before(session.ExpiresAt) {
		return nil, nil, db.DeleteSession(ctx, tokenHash)
	}

	user, err := db.UserByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, err
	}
	if !user.Active() {
		return nil, nil, db.DeleteSession(ctx, tokenHash)
	}

	return &session, user, nil
}

// ExtendSession moves a session's expiry forward. The request middleware only
// calls this once a day per session.
func (db *DB) ExtendSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, expiresAt.Unix(), tokenHash)
	return err
}

// DeleteSession removes one session.
func (db *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// ChangePassword updates a user's password and removes all of their sessions
// except keepTokenHash, which is normally the session making the change. The
// update only happens while the password hash is still expectedHash, which
// closes the race with another password change. It reports whether the password
// was changed.
func (db *DB) ChangePassword(ctx context.Context, userID int64, expectedHash, passwordHash, keepTokenHash string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE users SET password_hash = ? WHERE id = ? AND password_hash = ?`,
		passwordHash, userID, expectedHash,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected != 1 {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepTokenHash); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
