package db

import (
	"context"
	"slices"
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

	if _, err := database.DeleteComment(ctx, comment.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	first, err = database.NotificationByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("NotificationByID after comment delete: %v", err)
	}
	if first.BodyHTML != "" {
		t.Errorf("deleted comment snippet = %q, want empty", first.BodyHTML)
	}
	notifications, err = database.ListNotifications(ctx, alice.ID, 100, 0)
	if err != nil || len(notifications) != 1 || notifications[0].ID != mention.ID {
		t.Fatalf("ListNotifications after comment delete = %v, %v; want only the thread mention", notifications, err)
	}

	if _, err := database.DeleteThread(ctx, thread.ID, base.Add(6*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	mention, err = database.NotificationByID(ctx, mention.ID)
	if err != nil {
		t.Fatalf("NotificationByID after thread delete: %v", err)
	}
	if mention.BodyHTML != "" {
		t.Errorf("deleted thread snippet = %q, want empty", mention.BodyHTML)
	}
	notifications, err = database.ListNotifications(ctx, alice.ID, 100, 0)
	if err != nil || len(notifications) != 0 {
		t.Fatalf("ListNotifications after thread delete = %v, %v; want none", notifications, err)
	}
	if count, err := database.CountUnreadNotifications(ctx, alice.ID); err != nil || count != 0 {
		t.Errorf("CountUnreadNotifications after deletes = %d, %v; want 0, nil", count, err)
	}
}

// TestNotificationsHiddenForDeletedContent checks that notifications for a
// deleted thread or a deleted comment are left out of both the notification
// list and the unread count, while other notifications stay visible.
func TestNotificationsHiddenForDeletedContent(t *testing.T) {
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

	newThread := func(title string) *Thread {
		t.Helper()
		thread, err := database.CreateThread(ctx, alice.ID, title, "", "body", "<p>body</p>", base)
		if err != nil {
			t.Fatalf("CreateThread %q: %v", title, err)
		}
		return thread
	}

	// A notification for a comment that is later deleted.
	deletedCommentThread := newThread("deleted comment thread")
	deletedComment, err := database.CreateComment(ctx, deletedCommentThread.ID, nil, bob.ID, "reply", "<p>reply</p>", base)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	deletedCommentNotification, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationReply, deletedCommentThread.ID, &deletedComment.ID, base.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("CreateNotification deleted comment: %v", err)
	}

	// A thread that is deleted, with both a comment notification and a mention
	// in its body.
	deletedThread := newThread("deleted thread")
	deletedThreadComment, err := database.CreateComment(ctx, deletedThread.ID, nil, bob.ID, "reply", "<p>reply</p>", base)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	deletedThreadCommentNotification, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationReply, deletedThread.ID, &deletedThreadComment.ID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateNotification deleted thread comment: %v", err)
	}
	deletedThreadMention, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationMention, deletedThread.ID, nil, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("CreateNotification deleted thread mention: %v", err)
	}

	// A live thread notification that must survive both deletions. It is the
	// oldest row, so it also proves pagination is applied after filtering: a
	// caller asking for the first page gets it rather than deleted rows.
	liveThread := newThread("live thread")
	liveComment, err := database.CreateComment(ctx, liveThread.ID, nil, bob.ID, "reply", "<p>reply</p>", base)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	liveNotification, err := database.CreateNotification(ctx, alice.ID, bob.ID, NotificationReply, liveThread.ID, &liveComment.ID, base)
	if err != nil {
		t.Fatalf("CreateNotification live: %v", err)
	}

	wantInitial := []int64{deletedCommentNotification.ID, deletedThreadMention.ID, deletedThreadCommentNotification.ID, liveNotification.ID}
	if got := notificationIDs(t, database, alice.ID); !slices.Equal(got, wantInitial) {
		t.Fatalf("initial notifications = %v, want %v", got, wantInitial)
	}
	if count, err := database.CountUnreadNotifications(ctx, alice.ID); err != nil || count != 4 {
		t.Fatalf("initial unread count = %d, %v; want 4, nil", count, err)
	}

	if _, err := database.DeleteComment(ctx, deletedComment.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	want := []int64{deletedThreadMention.ID, deletedThreadCommentNotification.ID, liveNotification.ID}
	if got := notificationIDs(t, database, alice.ID); !slices.Equal(got, want) {
		t.Errorf("notifications after comment delete = %v, want %v", got, want)
	}
	if count, err := database.CountUnreadNotifications(ctx, alice.ID); err != nil || count != 3 {
		t.Errorf("unread count after comment delete = %d, %v; want 3, nil", count, err)
	}

	if _, err := database.DeleteThread(ctx, deletedThread.ID, base.Add(6*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if got := notificationIDs(t, database, alice.ID); !slices.Equal(got, []int64{liveNotification.ID}) {
		t.Errorf("notifications after thread delete = %v, want only %d", got, liveNotification.ID)
	}
	if count, err := database.CountUnreadNotifications(ctx, alice.ID); err != nil || count != 1 {
		t.Errorf("unread count after thread delete = %d, %v; want 1, nil", count, err)
	}

	// A page that starts at the newest rows must not be filled with deleted
	// notifications.
	notifications, err := database.ListNotifications(ctx, alice.ID, 2, 0)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifications) != 1 || notifications[0].ID != liveNotification.ID {
		t.Errorf("first page = %v, want only the live notification", notifications)
	}

	// The deleted rows still exist; they are hidden, not removed.
	if notification, err := database.NotificationByID(ctx, deletedThreadMention.ID); err != nil || notification == nil {
		t.Errorf("NotificationByID deleted thread mention = %v, %v; want the row", notification, err)
	}
}

func notificationIDs(t *testing.T, database *DB, recipientID int64) []int64 {
	t.Helper()
	notifications, err := database.ListNotifications(context.Background(), recipientID, 100, 0)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	ids := make([]int64, len(notifications))
	for i, notification := range notifications {
		ids[i] = notification.ID
	}
	return ids
}
