package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/benchanczh/shanji/internal/domain/planner"
	"github.com/benchanczh/shanji/internal/store"
)

type generatePlanRequest struct {
	WeekStart string `json:"week_start"`
}

type generatePlanResponse struct {
	Plan     *store.PlanView `json:"plan"`
	Warnings []string        `json:"warnings,omitempty"`
}

func (s *Server) handleGeneratePlan(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())

	var req generatePlanRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	weekStart, err := time.Parse("2006-01-02", req.WeekStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "week_start must be YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	h, err := s.households.Get(ctx, claims.HouseholdID)
	if err != nil {
		s.fail(w, "load household", err)
		return
	}
	recipes, err := s.planning.LoadActiveRecipes(ctx)
	if err != nil {
		s.fail(w, "load recipes", err)
		return
	}
	rules, err := s.planning.LoadHardRules(ctx, claims.HouseholdID)
	if err != nil {
		s.fail(w, "load rules", err)
		return
	}
	history, err := s.planning.LoadRecentMains(ctx, claims.HouseholdID, weekStart)
	if err != nil {
		s.fail(w, "load history", err)
		return
	}

	cfg := planner.Config{CuisineRatio: h.CuisineRatio}
	if h.PrimaryCuisine != nil {
		cfg.PrimaryCuisine = *h.PrimaryCuisine
	}
	if h.SecondaryCuisine != nil {
		cfg.SecondaryCuisine = *h.SecondaryCuisine
	}

	plan := planner.Generate(planner.Request{
		Recipes:        recipes,
		Rules:          rules,
		Config:         cfg,
		Tmpl:           parseTemplate(h.MealTemplate),
		HistoryDaysAgo: history,
		Days:           7,
		BabyMeals:      true,
		Seed:           time.Now().UnixNano(), // each regenerate explores a different variation
	})

	if _, err := s.planning.SavePlan(ctx, claims.HouseholdID, weekStart, plan); err != nil {
		if errors.Is(err, store.ErrPlanConfirmed) {
			writeError(w, http.StatusConflict, "plan for this week is already confirmed")
			return
		}
		s.fail(w, "save plan", err)
		return
	}

	view, err := s.planning.GetPlan(ctx, claims.HouseholdID, weekStart)
	if err != nil {
		s.fail(w, "load plan", err)
		return
	}
	writeJSON(w, http.StatusOK, generatePlanResponse{Plan: view, Warnings: plan.Warnings})
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	weekStart, err := time.Parse("2006-01-02", r.URL.Query().Get("week_start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "week_start query param must be YYYY-MM-DD")
		return
	}
	view, err := s.planning.GetPlan(r.Context(), claims.HouseholdID, weekStart)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no plan for this week")
		return
	}
	if err != nil {
		s.fail(w, "load plan", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// parseTemplate maps the household meal_template JSON to the planner
// template, falling back to the PRD default on any malformed input.
func parseTemplate(raw json.RawMessage) planner.Template {
	var doc map[string]struct {
		Main int    `json:"main"`
		Side int    `json:"side"`
		Soup string `json:"soup"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return planner.DefaultTemplate()
	}
	tmpl := planner.DefaultTemplate()
	if l, ok := doc["lunch"]; ok && l.Main > 0 {
		tmpl.Lunch = planner.MealSpec{Mains: l.Main, Sides: l.Side, Soup: l.Soup == "optional" || l.Soup == "required"}
	}
	if d, ok := doc["dinner"]; ok && d.Main > 0 {
		tmpl.Dinner = planner.MealSpec{Mains: d.Main, Sides: d.Side, Soup: d.Soup == "optional" || d.Soup == "required"}
	}
	return tmpl
}

func (s *Server) fail(w http.ResponseWriter, msg string, err error) {
	s.log.Error(msg, zap.Error(err))
	writeError(w, http.StatusInternalServerError, "internal server error")
}
