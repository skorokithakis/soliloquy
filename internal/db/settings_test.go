package db

import (
	"context"
	"reflect"
	"testing"
)

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()

	settings, err := database.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings on a fresh database: %v", err)
	}
	if settings.Public {
		t.Error("public defaults to on, want off")
	}
	if settings.ForumName != DefaultForumName {
		t.Errorf("forum name = %q, want %q", settings.ForumName, DefaultForumName)
	}
	if len(settings.DiscordWebhooks) != 0 {
		t.Errorf("discord webhooks = %v, want none", settings.DiscordWebhooks)
	}

	want := Settings{
		Public:          true,
		ForumName:       "The Forum",
		DiscordWebhooks: []string{"https://discord.com/api/webhooks/1/aaa", "https://discord.com/api/webhooks/2/bbb"},
	}
	if err := database.SetSettings(ctx, want); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}

	stored, err := database.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings after save: %v", err)
	}
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("settings = %+v, want %+v", stored, want)
	}

	updated := Settings{Public: false, ForumName: "Another Name"}
	if err := database.SetSettings(ctx, updated); err != nil {
		t.Fatalf("SetSettings update: %v", err)
	}
	stored, err = database.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings after update: %v", err)
	}
	if !reflect.DeepEqual(stored, updated) {
		t.Errorf("updated settings = %+v, want %+v", stored, updated)
	}
}

func TestSettingsDiscordWebhooksIgnoreBlankLines(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()

	if _, err := database.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)`,
		settingDiscordWebhooks, "\nhttps://discord.com/api/webhooks/1/aaa\n\n  https://discord.com/api/webhooks/2/bbb  \n",
	); err != nil {
		t.Fatalf("store webhooks: %v", err)
	}

	settings, err := database.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	want := []string{"https://discord.com/api/webhooks/1/aaa", "https://discord.com/api/webhooks/2/bbb"}
	if !reflect.DeepEqual(settings.DiscordWebhooks, want) {
		t.Errorf("discord webhooks = %v, want %v", settings.DiscordWebhooks, want)
	}
}
