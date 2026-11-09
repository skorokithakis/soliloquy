package db

import (
	"context"
	"testing"
	"time"
)

func TestNotificationLifecycle(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", base)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, bob.ID, "reply", "<p>reply</p>", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	first, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationReply, thread.ID, &comment.ID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	if first.ActorName() != "bob" || first.ThreadTitle != "Thread" || first.CommentID == nil || *first.CommentID != comment.ID {
		t.Errorf("notification = %+v", first)
	}
	if first.BodyHTML != "<p>reply</p>" {
		t.Errorf("reply notification body = %q, want comment HTML", first.BodyHTML)
	}
	if first.ReadAt != nil {
		t.Error("new notification is already read")
	}

	mention, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationMention, thread.ID, nil, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("CreateNotification mention: %v", err)
	}
	if mention.BodyHTML != "<p>body</p>" {
		t.Errorf("thread mention body = %q, want thread HTML", mention.BodyHTML)
	}

	if _, err := database.DeleteComment(ctx, comment.ID, base.Add(4*time.Minute)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	first, err = database.NotificationByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("NotificationByID after comment delete: %v", err)
	}
	if first.BodyHTML != "" {
		t.Errorf("deleted comment snippet = %q, want empty", first.BodyHTML)
	}

	if _, err := database.DeleteThread(ctx, thread.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	mention, err = database.NotificationByID(ctx, mention.ID)
	if err != nil {
		t.Fatalf("NotificationByID after thread delete: %v", err)
	}
	if mention.BodyHTML != "" {
		t.Errorf("deleted thread snippet = %q, want empty", mention.BodyHTML)
	}

	count, err := database.CountUnreadNotifications(ctx, alice.ID)
	if err != nil || count != 2 {
		t.Fatalf("CountUnreadNotifications = %d, %v; want 2, nil", count, err)
	}
	if count, err := database.CountUnreadNotifications(ctx, bob.ID); err != nil || count != 0 {
		t.Errorf("bob CountUnreadNotifications = %d, %v; want 0, nil", count, err)
	}

	notifications, err := database.ListNotifications(ctx, alice.ID, 100, 0)
	if err != nil || len(notifications) != 2 {
		t.Fatalf("ListNotifications = %v, %v; want 2", notifications, err)
	}
	if notifications[0].Kind != NotificationMention || notifications[1].Kind != NotificationReply {
		t.Errorf("notification order = %q, %q; want newest first", notifications[0].Kind, notifications[1].Kind)
	}

	if err := database.MarkNotificationsRead(ctx, alice.ID, base.Add(4*time.Minute)); err != nil {
		t.Fatalf("MarkNotificationsRead: %v", err)
	}
	count, err = database.CountUnreadNotifications(ctx, alice.ID)
	if err != nil || count != 0 {
		t.Fatalf("CountUnreadNotifications after mark = %d, %v; want 0, nil", count, err)
	}

	notifications, err = database.ListNotifications(ctx, alice.ID, 100, 0)
	if err != nil || len(notifications) != 2 {
		t.Fatalf("ListNotifications after mark = %v, %v; want 2", notifications, err)
	}
	for _, notification := range notifications {
		if notification.ReadAt == nil {
			t.Errorf("notification %d was not marked read", notification.ID)
		}
	}
}
