package db

import (
	"context"
	"time"
)

// FollowThread makes userID follow threadID. Following a thread twice is a
// no-op, so the first follow time is kept.
func (db *DB) FollowThread(ctx context.Context, userID, threadID int64, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO thread_follows (user_id, thread_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id, thread_id) DO NOTHING`,
		userID, threadID, now.Unix())
	return err
}

// UnfollowThread stops userID following threadID. Unfollowing a thread that is
// not followed is a no-op.
func (db *DB) UnfollowThread(ctx context.Context, userID, threadID int64) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM thread_follows WHERE user_id = ? AND thread_id = ?`,
		userID, threadID)
	return err
}

// FollowsThread reports whether userID follows threadID. Disabled and deleted
// members keep their follows, so this ignores account state.
func (db *DB) FollowsThread(ctx context.Context, userID, threadID int64) (bool, error) {
	var follows int64
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM thread_follows WHERE user_id = ? AND thread_id = ?
		)`, userID, threadID).Scan(&follows)
	if err != nil {
		return false, err
	}
	return follows != 0, nil
}

// ThreadFollowers returns the IDs of the active members who follow threadID.
// Disabled and deleted members are left out because they receive no
// notifications.
func (db *DB) ThreadFollowers(ctx context.Context, threadID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.user_id
		FROM thread_follows f
		JOIN users u ON u.id = f.user_id
		WHERE f.thread_id = ? AND u.disabled_at IS NULL AND u.deleted_at IS NULL
		ORDER BY f.user_id`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followers := []int64{}
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		followers = append(followers, userID)
	}
	return followers, rows.Err()
}
