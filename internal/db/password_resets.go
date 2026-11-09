package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrResetInvalid is returned for missing, used or expired password reset
// links.
var ErrResetInvalid = errors.New("password reset link is invalid or has expired")

// PasswordReset is a single-use link that lets a user set a new password.
type PasswordReset struct {
	Token     string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

const passwordResetColumns = `token, user_id, created_at, expires_at, used_at`

func scanPasswordReset(row rowScanner) (*PasswordReset, error) {
	var (
		reset     PasswordReset
		createdAt int64
		expiresAt int64
		usedAt    sql.NullInt64
	)
	if err := row.Scan(
		&reset.Token,
		&reset.UserID,
		&createdAt,
		&expiresAt,
		&usedAt,
	); err != nil {
		return nil, err
	}

	reset.CreatedAt = timeFromUnix(createdAt)
	reset.ExpiresAt = timeFromUnix(expiresAt)
	reset.UsedAt = nullTimePtr(usedAt)
	return &reset, nil
}

// CreatePasswordReset inserts a single-use reset link that expires after
// InviteLifetime, matching the rules for invite links.
func (db *DB) CreatePasswordReset(ctx context.Context, token string, userID int64, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO password_resets (token, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		token, userID, now.Unix(), now.Add(InviteLifetime).Unix(),
	)
	return err
}

// ActivePasswordReset returns an unused, unexpired password reset link. It
// returns ErrResetInvalid for missing, used or expired tokens.
func (db *DB) ActivePasswordReset(ctx context.Context, token string, now time.Time) (*PasswordReset, error) {
	reset, err := scanPasswordReset(db.QueryRowContext(ctx, `
		SELECT `+passwordResetColumns+` FROM password_resets WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrResetInvalid
	}
	if err != nil {
		return nil, err
	}
	if reset.UsedAt != nil || !now.Before(reset.ExpiresAt) {
		return nil, ErrResetInvalid
	}
	return reset, nil
}

// RedeemPasswordReset atomically marks a reset link used, changes the user's
// password and ends all of their sessions. The conditional update guarantees
// that a token works only once.
func (db *DB) RedeemPasswordReset(ctx context.Context, token, passwordHash string, now time.Time) (*User, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	reset, err := scanPasswordReset(tx.QueryRowContext(ctx, `
		SELECT `+passwordResetColumns+` FROM password_resets WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrResetInvalid
	}
	if err != nil {
		return nil, err
	}
	if reset.UsedAt != nil || !now.Before(reset.ExpiresAt) {
		return nil, ErrResetInvalid
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE password_resets SET used_at = ?
		WHERE token = ? AND used_at IS NULL`, now.Unix(), token)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrResetInvalid
	}

	result, err = tx.ExecContext(ctx, `
		UPDATE users SET password_hash = ?
		WHERE id = ? AND deleted_at IS NULL`, passwordHash, reset.UserID)
	if err != nil {
		return nil, err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrResetInvalid
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, reset.UserID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.UserByID(ctx, reset.UserID)
}
