package db

import (
	"context"
	"time"
)

// UserCounts is the number of accounts by state. Soft-deleted accounts are in
// neither state: their row only remains so the username stays reserved and
// old content keeps its author.
type UserCounts struct {
	Active   int64
	Disabled int64
}

// CountUsersByState returns the number of active and disabled accounts.
func (db *DB) CountUsersByState(ctx context.Context) (UserCounts, error) {
	var counts UserCounts
	if err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN disabled_at IS NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN disabled_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM users
		WHERE deleted_at IS NULL`).Scan(&counts.Active, &counts.Disabled); err != nil {
		return UserCounts{}, err
	}
	return counts, nil
}

// CountActiveUsersSince returns the number of non-disabled accounts that were
// active at or after since: they read a thread, started a thread or wrote a
// comment. There is no last-seen column, so the count is an estimate rather
// than a complete record of activity.
func (db *DB) CountActiveUsersSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM users u
		WHERE u.disabled_at IS NULL
		  AND u.deleted_at IS NULL
		  AND (
			EXISTS (
				SELECT 1 FROM thread_reads r
				WHERE r.user_id = u.id AND r.last_read_at >= ?
			)
			OR EXISTS (
				SELECT 1 FROM threads t
				WHERE t.author_id = u.id AND t.created_at >= ?
			)
			OR EXISTS (
				SELECT 1 FROM comments c
				WHERE c.author_id = u.id AND c.created_at >= ?
			)
		  )`, since.Unix(), since.Unix(), since.Unix()).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// CountComments returns the number of comments that are not soft-deleted.
func (db *DB) CountComments(ctx context.Context) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
