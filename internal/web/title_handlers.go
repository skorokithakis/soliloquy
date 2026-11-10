package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type titleResponse struct {
	Title string `json:"title"`
}

// handleTitle fetches the title of a page so the thread form can fill in the
// title when someone pastes a URL. Every failure is a 200 with an empty title,
// because the title is only a convenience and must never block the form.
func (s *Server) handleTitle(w http.ResponseWriter, r *http.Request) {
	title := ""
	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL != "" {
		if normalized, err := validateThreadURL(rawURL); err == nil {
			// Failures deliberately leave the title empty.
			title, _ = s.titleFetcher.Fetch(r.Context(), normalized)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(titleResponse{Title: title}); err != nil {
		s.serverError(w, err)
	}
}
