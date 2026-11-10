package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Errors returned by account and invite operations.
var (
	ErrInviteInvalid = errors.New("invite is invalid or has expired")
	ErrUsernameTaken = errors.New("username is already taken")
	ErrLastAdmin     = errors.New("the last active admin cannot delete their account")
)

// User is an account. Usernames are stored in their canonical lowercase form.
// A soft-deleted user keeps the row so the username stays reserved and content
// keeps its author.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	IsAdmin      bool
	InvitedBy    *int64
	CreatedAt    time.Time
	DisabledAt   *time.Time
	DeletedAt    *time.Time
	Theme        string
	// FeedToken is the secret in the user's RSS feed URLs. It is empty until
	// the user first opens their account page.
	FeedToken string
}

// Active reports whether the user may log in and use the site.
func (u *User) Active() bool {
	return u != nil && u.DisabledAt == nil && u.DeletedAt == nil
}

// DisplayName is the name shown for the user, including on content authored
// before an account was deleted.
func (u *User) DisplayName() string {
	if u == nil || u.DeletedAt != nil {
		return "[deleted user]"
	}
	return u.Username
}

const userColumns = `id, username, password_hash, is_admin, invited_by, created_at, disabled_at, deleted_at, theme, feed_token`

type rowScanner interface {
	Scan(dest ...any) error
}

type execQueryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func scanUser(row rowScanner) (*User, error) {
	var (
		user       User
		isAdmin    int64
		invitedBy  sql.NullInt64
		createdAt  int64
		disabledAt sql.NullInt64
		deletedAt  sql.NullInt64
		feedToken  sql.NullString
	)
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&isAdmin,
		&invitedBy,
		&createdAt,
		&disabledAt,
		&deletedAt,
		&user.Theme,
		&feedToken,
	); err != nil {
		return nil, err
	}

	user.IsAdmin = isAdmin != 0
	user.InvitedBy = nullInt64Ptr(invitedBy)
	user.CreatedAt = timeFromUnix(createdAt)
	user.DisabledAt = nullTimePtr(disabledAt)
	user.DeletedAt = nullTimePtr(deletedAt)
	user.FeedToken = feedToken.String
	return &user, nil
}

// UserByID returns a user, or nil when no user has that ID.
func (db *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	user, err := scanUser(db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UserByUsername looks up a user by username. The comparison is
// case-insensitive because of the column's NOCASE collation, but callers
// should normalize first.
func (db *DB) UserByUsername(ctx context.Context, username string) (*User, error) {
	user, err := scanUser(db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UserByFeedToken returns the user whose feed token matches, or nil when no
// user has that token. Callers must check Active before granting access.
func (db *DB) UserByFeedToken(ctx context.Context, token string) (*User, error) {
	user, err := scanUser(db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE feed_token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// SetFeedToken stores a user's feed token, replacing the previous one. Callers
// generate the token, so a reset is just another call with a new value.
func (db *DB) SetFeedToken(ctx context.Context, userID int64, token string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET feed_token = ? WHERE id = ?`, token, userID)
	return err
}

// SetFeedTokenIfEmpty stores token only when the user has no feed token yet,
// then returns the token that is stored. Concurrent first calls therefore
// agree on one token instead of overwriting each other.
func (db *DB) SetFeedTokenIfEmpty(ctx context.Context, userID int64, token string) (string, error) {
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET feed_token = ? WHERE id = ? AND feed_token IS NULL`,
		token, userID,
	); err != nil {
		return "", err
	}

	var stored sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT feed_token FROM users WHERE id = ?`, userID).Scan(&stored); err != nil {
		return "", err
	}
	return stored.String, nil
}

// SetTheme stores a user's theme preference.
func (db *DB) SetTheme(ctx context.Context, userID int64, theme string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET theme = ? WHERE id = ?`, theme, userID)
	return err
}

// CreateUser inserts a user. invitedBy is nil for the first admin; normal
// users record who invited them.
func (db *DB) CreateUser(ctx context.Context, username, passwordHash string, isAdmin bool, invitedBy *int64, now time.Time) (*User, error) {
	return createUser(ctx, db, username, passwordHash, isAdmin, invitedBy, now)
}

func createUser(ctx context.Context, q execQueryer, username, passwordHash string, isAdmin bool, invitedBy *int64, now time.Time) (*User, error) {
	result, err := q.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, is_admin, invited_by, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		username, passwordHash, boolInt(isAdmin), nullableInt64(invitedBy), now.Unix(),
	)
	if err != nil {
		return nil, userInsertError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return scanUser(q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// ListUsers returns every account, including disabled and deleted ones,
// oldest first. The invite tree uses this order for siblings.
func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+userColumns+` FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

// CountInvitedUsers returns how many accounts were invited by userID,
// including accounts that were later deleted.
func (db *DB) CountInvitedUsers(ctx context.Context, userID int64) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users WHERE invited_by = ?`, userID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// InviteNode is a user and the users they invited.
type InviteNode struct {
	User     User
	Children []*InviteNode
}

// BuildInviteTree nests users under the account that invited them. Deleted
// users remain in the tree because their invitees keep their place. The first
// admin has no inviter and is a root; unrelated accounts with no inviter are
// also roots.
func BuildInviteTree(users []User) []*InviteNode {
	nodes := make(map[int64]*InviteNode, len(users))
	for i := range users {
		nodes[users[i].ID] = &InviteNode{User: users[i]}
	}

	roots := make([]*InviteNode, 0, len(users))
	for i := range users {
		node := nodes[users[i].ID]
		if users[i].InvitedBy == nil {
			roots = append(roots, node)
			continue
		}
		parent, ok := nodes[*users[i].InvitedBy]
		if !ok {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	return roots
}

func userInsertError(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
		return ErrUsernameTaken
	}
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nullTimePtr(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := timeFromUnix(value.Int64)
	return &t
}

func timeFromUnix(value int64) time.Time {
	return time.Unix(value, 0).UTC()
}
