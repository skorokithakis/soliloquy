package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DeleteAccount soft-deletes a user's account and removes the private data
// that should not outlive it: sessions, notifications and unused invites.
// Threads, comments and votes stay so the forum keeps its history. The row
// stays, which also keeps the username reserved and lets content show
// "[deleted user]".
//
// The last active admin cannot delete their account; ErrLastAdmin is returned
// in that case.
func (db *DB) DeleteAccount(ctx context.Context, userID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		isAdmin    int64
		disabledAt sql.NullInt64
		deletedAt  sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT is_admin, disabled_at, deleted_at FROM users WHERE id = ?`, userID,
	).Scan(&isAdmin, &disabledAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	if deletedAt.Valid {
		return nil
	}

	// Only an active admin can be the last active admin.
	if isAdmin != 0 && !disabledAt.Valid {
		if err := ensureOtherActiveAdmin(ctx, tx, userID); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		now.Unix(), userID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE recipient_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM invites WHERE created_by = ? AND used_at IS NULL`, userID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM password_resets WHERE user_id = ? AND used_at IS NULL`, userID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// DisableUser stops a user from logging in, ends their sessions and cancels
// their unused invites. It reports whether the account was newly disabled. The
// last active admin cannot be disabled; ErrLastAdmin is returned in that case.
func (db *DB) DisableUser(ctx context.Context, userID int64, now time.Time) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var (
		isAdmin    int64
		disabledAt sql.NullInt64
		deletedAt  sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT is_admin, disabled_at, deleted_at FROM users WHERE id = ?`, userID,
	).Scan(&isAdmin, &disabledAt, &deletedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && isAdmin != 0 && !disabledAt.Valid && !deletedAt.Valid {
		if err := ensureOtherActiveAdmin(ctx, tx, userID); err != nil {
			return false, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users SET disabled_at = ?
		WHERE id = ? AND disabled_at IS NULL AND deleted_at IS NULL`,
		now.Unix(), userID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM invites WHERE created_by = ? AND used_at IS NULL`, userID,
	); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected == 1, nil
}

// SetAdmin grants or removes a user's admin rights. Deleted accounts are left
// alone. The caller enforces that an admin cannot remove their own rights, and
// the last active admin cannot lose theirs; ErrLastAdmin is returned in that
// case.
func (db *DB) SetAdmin(ctx context.Context, userID int64, isAdmin bool) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var (
		currentAdmin int64
		disabledAt   sql.NullInt64
		deletedAt    sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT is_admin, disabled_at, deleted_at FROM users WHERE id = ?`, userID,
	).Scan(&currentAdmin, &disabledAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if deletedAt.Valid {
		return false, nil
	}

	if !isAdmin && currentAdmin != 0 && !disabledAt.Valid {
		if err := ensureOtherActiveAdmin(ctx, tx, userID); err != nil {
			return false, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users SET is_admin = ?
		WHERE id = ? AND deleted_at IS NULL`, boolInt(isAdmin), userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ensureOtherActiveAdmin returns ErrLastAdmin when userID is the only active
// admin. Callers run it inside the transaction doing the mutation.
func ensureOtherActiveAdmin(ctx context.Context, q execQueryer, userID int64) error {
	var otherAdmins int64
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		WHERE id <> ? AND is_admin = 1
			AND disabled_at IS NULL AND deleted_at IS NULL`, userID,
	).Scan(&otherAdmins); err != nil {
		return err
	}
	if otherAdmins == 0 {
		return ErrLastAdmin
	}
	return nil
}
