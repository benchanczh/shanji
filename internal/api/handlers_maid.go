package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/benchanczh/shanji/internal/store"
)

// requireMaidToken guards the helper-facing endpoints: a valid,
// unrevoked magic link token resolves to a household ID. The token is
// the only credential — scope is inherently limited to the routes
// that use this middleware (today view + suggestions).
func (s *Server) requireMaidToken(next func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing token")
			return
		}
		householdID, err := s.maid.HouseholdForToken(r.Context(), token)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid or revoked link")
			return
		}
		if err != nil {
			s.fail(w, "resolve maid token", err)
			return
		}
		next(w, r, householdID)
	}
}

// POST /api/v1/maid-link (decider) — issue a fresh magic link,
// revoking any previous one.
func (s *Server) handleCreateMaidLink(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	token, err := s.maid.CreateLink(r.Context(), claims.HouseholdID)
	if err != nil {
		s.fail(w, "create maid link", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
		"path":  "/maid/today?token=" + token,
	})
}

// DELETE /api/v1/maid-link (decider) — revoke all active links.
func (s *Server) handleRevokeMaidLink(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	if err := s.maid.RevokeLinks(r.Context(), claims.HouseholdID); err != nil {
		s.fail(w, "revoke maid link", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// GET /api/v1/maid/today?token=&date= — what to cook today, bilingual.
func (s *Server) handleMaidToday(w http.ResponseWriter, r *http.Request, householdID int64) {
	// Default to the server's local calendar date, normalized to a
	// timezone-free UTC midnight so the SQL DATE comparison is exact.
	// (time.Truncate(24h) cuts at UTC day boundaries and shifts the
	// date for non-UTC servers.)
	now := time.Now()
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if d := r.URL.Query().Get("date"); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		date = parsed
	}
	view, err := s.maid.TodayView(r.Context(), householdID, date)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no confirmed plan covers this date")
		return
	}
	if err != nil {
		s.fail(w, "load today view", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type suggestionRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// POST /api/v1/maid/suggestions?token= — helper proposes a dish.
func (s *Server) handleMaidSuggest(w http.ResponseWriter, r *http.Request, householdID int64) {
	var req suggestionRequest
	if err := decodeBody(r, &req); err != nil || req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	id, err := s.maid.CreateSuggestion(r.Context(), householdID, "helper", req.Title, req.Content)
	if err != nil {
		s.fail(w, "create suggestion", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending"})
}

// GET /api/v1/suggestions?status= (decider)
func (s *Server) handleListSuggestions(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	status := r.URL.Query().Get("status")
	if status != "" && status != "pending" && status != "approved" && status != "rejected" {
		writeError(w, http.StatusBadRequest, "invalid status filter")
		return
	}
	list, err := s.maid.ListSuggestions(r.Context(), claims.HouseholdID, status)
	if err != nil {
		s.fail(w, "list suggestions", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/v1/suggestions/{id}/approve | /reject (decider) — the
// two-button review from the PRD.
func (s *Server) handleReviewSuggestion(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFrom(r.Context())
		id, ok := pathID(r, "id")
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid suggestion id")
			return
		}
		err := s.maid.SetSuggestionStatus(r.Context(), claims.HouseholdID, id, status)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "suggestion not found or already reviewed")
			return
		}
		if err != nil {
			s.fail(w, "review suggestion", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
	}
}
