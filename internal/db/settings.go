package db

import "context"

// DefaultForumName is the forum name when no setting is stored.
const DefaultForumName = "soliloquy"

const (
	settingPublic    = "public"
	settingForumName = "forum_name"
)

// Settings holds the site-wide settings admins can change.
type Settings struct {
	Public    bool
	ForumName string
}

// Settings returns the stored site settings. Missing values use their
// defaults, so this never fails because a setting was never saved.
func (db *DB) Settings(ctx context.Context) (Settings, error) {
	settings := Settings{ForumName: DefaultForumName}

	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, err
		}
		switch key {
		case settingPublic:
			settings.Public = value == "1"
		case settingForumName:
			settings.ForumName = value
		}
	}
	if err := rows.Err(); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// SetSettings stores the site settings, replacing any previous values.
func (db *DB) SetSettings(ctx context.Context, settings Settings) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	public := "0"
	if settings.Public {
		public = "1"
	}

	for key, value := range map[string]string{
		settingPublic:    public,
		settingForumName: settings.ForumName,
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
