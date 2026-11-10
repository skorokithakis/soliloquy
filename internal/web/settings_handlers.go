package web

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/skorokithakis/soliloquy/internal/db"
)

const maxForumNameLength = 40

type adminSettingsPageData struct {
	PageData
	Public          bool
	ForumName       string
	DiscordWebhooks string
	// DiscordTestResult is set after the admin sends a test message.
	DiscordTestResult string
}

func (s *Server) handleAdminSettingsPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	settings, err := s.db.Settings(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.render(w, "admin_settings", adminSettingsPageData{
		PageData:        s.pageData(r, "Admin settings"),
		Public:          settings.Public,
		ForumName:       settings.ForumName,
		DiscordWebhooks: strings.Join(settings.DiscordWebhooks, "\n"),
	})
}

func (s *Server) handleAdminSettingsSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	settings := db.Settings{
		Public:    r.FormValue("public") != "",
		ForumName: strings.TrimSpace(r.FormValue("forum_name")),
	}
	if !validForumName(settings.ForumName) {
		s.renderStatus(w, http.StatusBadRequest, "admin_settings", adminSettingsPageData{
			PageData:        s.pageDataWithError(r, "Admin settings", "Forum name must be between 1 and 40 characters."),
			Public:          settings.Public,
			ForumName:       settings.ForumName,
			DiscordWebhooks: r.FormValue("discord_webhooks"),
		})
		return
	}

	webhooks, err := validateDiscordWebhooks(r.FormValue("discord_webhooks"))
	if err != nil {
		s.renderStatus(w, http.StatusBadRequest, "admin_settings", adminSettingsPageData{
			PageData:        s.pageDataWithError(r, "Admin settings", err.Error()),
			Public:          settings.Public,
			ForumName:       settings.ForumName,
			DiscordWebhooks: r.FormValue("discord_webhooks"),
		})
		return
	}
	settings.DiscordWebhooks = webhooks

	if err := s.db.SetSettings(r.Context(), settings); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
}

func validForumName(name string) bool {
	length := utf8.RuneCountInString(name)
	return length > 0 && length <= maxForumNameLength
}
