package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/discord"
)

// discordTestMessage is what the "send test message" button posts.
const discordTestMessage = "Test message from the forum. If you can read this, the Discord webhook works."

// webhookSender posts plain-text messages to webhook URLs. It is an interface
// so tests can avoid the network.
type webhookSender interface {
	Send(ctx context.Context, webhookURL, content string) error
}

// notifyDiscordThread posts a new thread to every configured webhook. It runs
// in the background after the thread is created, so it must not use the
// request context: that is cancelled as soon as the handler returns. Failures
// are logged without the webhook URL, which is a secret.
func (s *Server) notifyDiscordThread(thread *db.Thread) {
	settings, err := s.db.Settings(context.Background())
	if err != nil {
		slog.Error("load discord webhooks", "error", err)
		return
	}
	if len(settings.DiscordWebhooks) == 0 {
		return
	}

	content := discord.NewThreadContent(discord.ThreadMessage{
		Title:       thread.Title,
		Link:        strings.TrimRight(s.cfg.BaseURL, "/") + threadPath(thread.PublicID),
		Author:      thread.AuthorName(),
		ExternalURL: thread.URL,
		Body:        thread.Body,
	})
	for index, webhook := range settings.DiscordWebhooks {
		go func() {
			if err := s.webhooks.Send(context.Background(), webhook, content); err != nil {
				slog.Error("send discord webhook", "webhook", index+1, "error", err)
			}
		}()
	}
}

func (s *Server) handleAdminDiscordTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	settings, err := s.db.Settings(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.render(w, "admin_settings", adminSettingsPageData{
		PageData:          s.pageData(r, "Admin settings"),
		Public:            settings.Public,
		ForumName:         settings.ForumName,
		DiscordWebhooks:   strings.Join(settings.DiscordWebhooks, "\n"),
		DiscordTestResult: s.sendDiscordTest(r.Context(), settings.DiscordWebhooks),
	})
}

// sendDiscordTest sends the test message to the saved webhooks and describes
// the outcome, naming failed webhooks by their position in the list. The admin
// waits for it, so it runs synchronously.
func (s *Server) sendDiscordTest(ctx context.Context, webhooks []string) string {
	if len(webhooks) == 0 {
		return "No webhook URLs are configured, so nothing was sent."
	}

	var failed []string
	for index, webhook := range webhooks {
		if err := s.webhooks.Send(ctx, webhook, discordTestMessage); err != nil {
			slog.Error("send discord test message", "webhook", index+1, "error", err)
			failed = append(failed, fmt.Sprintf("#%d (%s)", index+1, discordFailureReason(err)))
		}
	}

	if len(failed) == 0 {
		return fmt.Sprintf("Test message sent to all %d webhooks.", len(webhooks))
	}
	return "Test message failed for webhook " + strings.Join(failed, ", ") + "."
}

// discordFailureReason describes an error without repeating the webhook URL.
func discordFailureReason(err error) string {
	var statusErr *discord.StatusError
	if errors.As(err, &statusErr) {
		return "HTTP " + strconv.Itoa(statusErr.Code)
	}
	return err.Error()
}

// validateDiscordWebhooks trims the textarea value and checks that every
// remaining line is an absolute https URL with a host.
func validateDiscordWebhooks(value string) ([]string, error) {
	var webhooks []string
	for index, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parsed, err := url.Parse(line)
		if err != nil || strings.ToLower(parsed.Scheme) != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("Line %d (%q) is not an https webhook URL.", index+1, line)
		}
		webhooks = append(webhooks, line)
	}
	return webhooks, nil
}
