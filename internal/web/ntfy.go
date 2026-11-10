package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/ntfy"
)

// ntfyTestMessage is what the "send test" button posts.
const ntfyTestMessage = "Test push from the forum. If you can read this, your ntfy topic works."

// pushSender posts plain-text push notifications. It is an interface so tests
// can avoid the network.
type pushSender interface {
	Send(ctx context.Context, topicURL string, message ntfy.Message) error
}

// wantsNotificationPush reports whether a user asked for pushes about their
// personal notifications. Inactive users never get pushes.
func wantsNotificationPush(user *db.User) bool {
	return user != nil && user.Active() && user.NtfyURL != "" && user.NtfyReplies
}

// wantsThreadPush reports whether a user asked for pushes about every new
// thread. The thread's own author is never notified.
func wantsThreadPush(user *db.User, authorID int64) bool {
	return user != nil && user.ID != authorID && user.Active() && user.NtfyURL != "" && user.NtfyThreads
}

// pushTitle is the Title header of every push: the forum name, so the phone
// shows which site the message came from.
func (s *Server) pushTitle(ctx context.Context) string {
	settings, err := s.db.Settings(ctx)
	if err != nil {
		slog.Error("load forum name for push", "error", err)
		return ""
	}
	return settings.ForumName
}

// pushNotification sends a personal notification to the recipient's ntfy
// topic. Callers start it in a goroutine with a background context, so a slow
// or failing push neither delays nor fails the request. Failures are logged
// without the topic URL, which is a secret.
func (s *Server) pushNotification(notification *db.Notification) {
	ctx := context.Background()
	user, err := s.db.UserByID(ctx, notification.RecipientID)
	if err != nil {
		slog.Error("load push recipient", "user_id", notification.RecipientID, "error", err)
		return
	}
	if !wantsNotificationPush(user) {
		return
	}

	message := ntfy.Message{
		Title: s.pushTitle(ctx),
		Text:  notificationText(notification),
		Click: s.absoluteURL(notificationPath(notification.ThreadPublicID, notification.CommentPublicID)),
	}
	if err := s.pushes.Send(ctx, user.NtfyURL, message); err != nil {
		slog.Error("send push notification", "user_id", user.ID, "error", err)
	}
}

// notifyNtfyThread sends a push for a new thread to every user who asked for
// them. Like the Discord notification it runs in the background, so it must
// not use the request context.
func (s *Server) notifyNtfyThread(thread *db.Thread) {
	ctx := context.Background()
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		slog.Error("load push recipients", "error", err)
		return
	}

	message := ntfy.Message{
		Title: s.pushTitle(ctx),
		Text:  ntfy.NewThreadText(thread.Title, thread.AuthorName()),
		Click: s.absoluteURL(threadPath(thread.PublicID)),
	}
	for _, user := range users {
		if !wantsThreadPush(&user, thread.AuthorID) {
			continue
		}
		s.goBackground(func() {
			if err := s.pushes.Send(ctx, user.NtfyURL, message); err != nil {
				slog.Error("send new thread push", "user_id", user.ID, "error", err)
			}
		})
	}
}

// handleSaveNotificationSettings stores the ntfy form. The URL is a secret, so
// errors never repeat it.
func (s *Server) handleSaveNotificationSettings(w http.ResponseWriter, r *http.Request) {
	values := ntfyFormValues{
		URL:     strings.TrimSpace(r.FormValue("ntfy_url")),
		Replies: r.FormValue("ntfy_replies") != "",
		Threads: r.FormValue("ntfy_threads") != "",
	}

	topicURL, err := validateNtfyURL(values.URL)
	if err != nil {
		s.renderAccountNtfy(w, r, http.StatusBadRequest, values, err.Error(), "")
		return
	}

	if err := s.db.SetNtfySettings(r.Context(), currentUser(r).ID, topicURL, values.Replies, values.Threads); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// handleNtfyTest sends a test push to the saved topic URL. The user waits for
// it, so it runs synchronously and shows the outcome on the account page.
func (s *Server) handleNtfyTest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	result := s.sendNtfyTest(r.Context(), user)

	data, err := s.accountPageData(r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	data.NtfyTestResult = result
	s.render(w, "account", data)
}

// sendNtfyTest posts the test message to the saved URL and describes the
// outcome. The result never contains the URL.
func (s *Server) sendNtfyTest(ctx context.Context, user *db.User) string {
	if user.NtfyURL == "" {
		return "No ntfy topic URL is saved, so nothing was sent."
	}

	message := ntfy.Message{
		Title: s.pushTitle(ctx),
		Text:  ntfyTestMessage,
		Click: s.absoluteURL("/account"),
	}
	if err := s.pushes.Send(ctx, user.NtfyURL, message); err != nil {
		slog.Error("send ntfy test push", "user_id", user.ID, "error", err)
		return "Test push failed: " + ntfyFailureReason(err) + "."
	}
	return "Test push sent to your ntfy topic."
}

// ntfyFailureReason describes an error without repeating the topic URL.
func ntfyFailureReason(err error) string {
	var statusErr *ntfy.StatusError
	if errors.As(err, &statusErr) {
		return "HTTP " + strconv.Itoa(statusErr.Code)
	}
	return err.Error()
}

// validateNtfyURL trims the value and checks that it is an absolute https URL
// with a host, or empty to turn pushes off. Any host is allowed because users
// may run their own ntfy server. The URL is a secret, so the error never
// repeats it.
func validateNtfyURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	parsed, err := url.Parse(value)
	if err != nil || strings.ToLower(parsed.Scheme) != "https" || parsed.Host == "" {
		return "", errors.New("Enter an absolute https ntfy topic URL, or leave it empty to turn pushes off.")
	}
	return value, nil
}
