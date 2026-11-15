package db

import (
	"context"
	"database/sql"
	"time"
)

// MarkThreadRead records that userID opened threadID at now and returns the
// cutoff that applied before the update. A comment counts as new for the user
// when it was created after that cutoff.
//
// The cutoff is the later of the previous last_read_at, when the user had
// opened the thread before, and the user's unread_since. A member who has never
// opened the thread therefore starts from signup or from the migration that
// introduced read state, which keeps older threads from counting as unread.
func (db *DB) MarkThreadRead(ctx context.Context, userID, threadID int64, now time.Time) (time.Time, error) {
	// No transaction: a read-then-write transaction can fail with
	// SQLITE_BUSY_SNAPSHOT in WAL mode. The only race is the same member opening
	// the same thread twice at once, and the worst outcome is a marker shown or
	// hidden once.
	var (
		unreadSince int64
		lastReadAt  sql.NullInt64
	)
	if err := db.QueryRowContext(ctx, `
		SELECT u.unread_since, r.last_read_at
		FROM users u
		LEFT JOIN thread_reads r ON r.user_id = u.id AND r.thread_id = ?
		WHERE u.id = ?`, threadID, userID).Scan(&unreadSince, &lastReadAt); err != nil {
		return time.Time{}, err
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO thread_reads (user_id, thread_id, last_read_at)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id, thread_id) DO UPDATE
		SET last_read_at = excluded.last_read_at`,
		userID, threadID, now.Unix()); err != nil {
		return time.Time{}, err
	}

	cutoff := unreadSince
	if lastReadAt.Valid && lastReadAt.Int64 > cutoff {
		cutoff = lastReadAt.Int64
	}
	return timeFromUnix(cutoff), nil
}
