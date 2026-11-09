package db

import (
	"context"
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

	want := Settings{Public: true, ForumName: "The Forum"}
	if err := database.SetSettings(ctx, want); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}

	stored, err := database.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings after save: %v", err)
	}
	if stored != want {
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
	if stored != updated {
		t.Errorf("updated settings = %+v, want %+v", stored, updated)
	}
}
