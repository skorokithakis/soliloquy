package db

import (
	"context"
	"time"
)

// MarkThreadRead records that userID opened threadID at now and returns the
// cutoff that applied before the update. A comment counts as new for the user
// when it was created after that cutoff.
//
// When the user has opened the thread before, the cutoff is the previous
// last_read_at. On the first visit there is no earlier visit to compare with,
// so the cutoff is now: the visit marks no comment as new, and only comments
// that arrive afterwards count on the next visit.
func (db *DB) MarkThreadRead(ctx context.Context, userID, threadID int64, now time.Time) (time.Time, error) {
	// No transaction: a read-then-write transaction can fail with
	// SQLITE_BUSY_SNAPSHOT in WAL mode. The only race is the same member opening
	// the same thread twice at once, and the worst outcome is a marker shown or
	// hidden once.
	var cutoff int64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(last_read_at), ?)
		FROM thread_reads
		WHERE user_id = ? AND thread_id = ?`,
		now.Unix(), userID, threadID).Scan(&cutoff); err != nil {
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

	return timeFromUnix(cutoff), nil
}
