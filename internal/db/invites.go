package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// InviteLifetime is how long invite and reset links stay valid.
const InviteLifetime = 7 * 24 * time.Hour

// Invite is a single-use account creation link. CreatedBy is nil for the
// bootstrap admin invite that is printed when the database has no users.
type Invite struct {
	Token         string
	CreatedBy     *int64
	IsAdmin       bool
	CreatedAt     time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
	UsedBy        *int64
	UsedByName    string
	UsedByDeleted bool
}

const inviteColumns = `token, created_by, is_admin, created_at, expires_at, used_at, used_by`

func scanInvite(row rowScanner) (*Invite, error) {
	var (
		invite    Invite
		createdBy sql.NullInt64
		isAdmin   int64
		createdAt int64
		expiresAt int64
		usedAt    sql.NullInt64
		usedBy    sql.NullInt64
	)
	if err := row.Scan(
		&invite.Token,
		&createdBy,
		&isAdmin,
		&createdAt,
		&expiresAt,
		&usedAt,
		&usedBy,
	); err != nil {
		return nil, err
	}

	invite.CreatedBy = nullInt64Ptr(createdBy)
	invite.IsAdmin = isAdmin != 0
	invite.CreatedAt = timeFromUnix(createdAt)
	invite.ExpiresAt = timeFromUnix(expiresAt)
	invite.UsedAt = nullTimePtr(usedAt)
	invite.UsedBy = nullInt64Ptr(usedBy)
	return &invite, nil
}

// CreateInvite inserts a single-use invite that expires after InviteLifetime.
func (db *DB) CreateInvite(ctx context.Context, token string, createdBy *int64, isAdmin bool, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO invites (token, created_by, is_admin, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		token, nullableInt64(createdBy), boolInt(isAdmin), now.Unix(), now.Add(InviteLifetime).Unix(),
	)
	return err
}

// ActiveInvite returns an unused, unexpired invite. It returns
// ErrInviteInvalid for missing, used or expired tokens.
func (db *DB) ActiveInvite(ctx context.Context, token string, now time.Time) (*Invite, error) {
	invite, err := scanInvite(db.QueryRowContext(ctx, `
		SELECT `+inviteColumns+` FROM invites WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, err
	}
	if invite.UsedAt != nil || !now.Before(invite.ExpiresAt) {
		return nil, ErrInviteInvalid
	}
	return invite, nil
}

// ListActiveInvites returns a user's unused, unexpired invites, newest first.
func (db *DB) ListActiveInvites(ctx context.Context, userID int64, now time.Time) ([]Invite, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+inviteColumns+` FROM invites
		WHERE created_by = ? AND used_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC, token`, userID, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := []Invite{}
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, *invite)
	}
	return invites, rows.Err()
}

// ListInvites returns all of a user's invites, newest first, including used
// and expired ones. UsedByName is set when the invite was redeemed.
func (db *DB) ListInvites(ctx context.Context, userID int64) ([]Invite, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT i.token, i.created_by, i.is_admin, i.created_at, i.expires_at,
			i.used_at, i.used_by, u.username, u.deleted_at
		FROM invites i
		LEFT JOIN users u ON u.id = i.used_by
		WHERE i.created_by = ?
		ORDER BY i.created_at DESC, i.token`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := []Invite{}
	for rows.Next() {
		var (
			invite     Invite
			createdBy  sql.NullInt64
			isAdmin    int64
			createdAt  int64
			expiresAt  int64
			usedAt     sql.NullInt64
			usedBy     sql.NullInt64
			usedByName sql.NullString
			usedByGone sql.NullInt64
		)
		if err := rows.Scan(
			&invite.Token,
			&createdBy,
			&isAdmin,
			&createdAt,
			&expiresAt,
			&usedAt,
			&usedBy,
			&usedByName,
			&usedByGone,
		); err != nil {
			return nil, err
		}

		invite.CreatedBy = nullInt64Ptr(createdBy)
		invite.IsAdmin = isAdmin != 0
		invite.CreatedAt = timeFromUnix(createdAt)
		invite.ExpiresAt = timeFromUnix(expiresAt)
		invite.UsedAt = nullTimePtr(usedAt)
		invite.UsedBy = nullInt64Ptr(usedBy)
		invite.UsedByName = usedByName.String
		invite.UsedByDeleted = usedByGone.Valid
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// RedeemInvite atomically creates a user from an invite and marks the invite
// used. The conditional update guarantees that a token works only once.
func (db *DB) RedeemInvite(ctx context.Context, token, username, passwordHash string, now time.Time) (*User, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invite, err := scanInvite(tx.QueryRowContext(ctx, `
		SELECT `+inviteColumns+` FROM invites WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, err
	}
	if invite.UsedAt != nil || !now.Before(invite.ExpiresAt) {
		return nil, ErrInviteInvalid
	}

	user, err := createUser(ctx, tx, username, passwordHash, invite.IsAdmin, invite.CreatedBy, now)
	if err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE invites SET used_at = ?, used_by = ?
		WHERE token = ? AND used_at IS NULL`, now.Unix(), user.ID, token)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrInviteInvalid
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return user, nil
}

// EnsureBootstrapInvite creates the first admin invite when the database has
// no users. It reports whether a new invite was created. Any older unused
// bootstrap invite is removed so only the URL just printed to the log works.
func (db *DB) EnsureBootstrapInvite(ctx context.Context, token string, now time.Time) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var users int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return false, err
	}
	if users > 0 {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invites WHERE created_by IS NULL AND used_at IS NULL`); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO invites (token, created_by, is_admin, created_at, expires_at)
		VALUES (?, NULL, 1, ?, ?)`,
		token, now.Unix(), now.Add(InviteLifetime).Unix(),
	); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
