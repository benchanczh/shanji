package api

import (
	"errors"
	"net/http"
	"slices"

	"github.com/benchanczh/shanji/internal/store"
)

// ---- members ----

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	members, err := s.family.ListMembers(r.Context(), claims.HouseholdID)
	if err != nil {
		s.fail(w, "list members", err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

type memberRequest struct {
	Name string `json:"name"`
	Age  *int   `json:"age"`
	Role string `json:"role"`
}

var memberRoles = []string{"decider", "spouse", "child", "helper"}

func (s *Server) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	var req memberRequest
	if err := decodeBody(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !slices.Contains(memberRoles, req.Role) {
		writeError(w, http.StatusBadRequest, "role must be one of decider/spouse/child/helper")
		return
	}
	id, err := s.family.CreateMember(r.Context(), claims.HouseholdID, req.Name, req.Age, req.Role)
	if err != nil {
		s.fail(w, "create member", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleDeleteMember(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid member id")
		return
	}
	err := s.family.DeleteMember(r.Context(), claims.HouseholdID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		s.fail(w, "delete member", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---- diet rules ----

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	rules, err := s.family.ListRules(r.Context(), claims.HouseholdID)
	if err != nil {
		s.fail(w, "list rules", err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

type ruleRequest struct {
	MemberID   *int64  `json:"member_id"`
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Ingredient string  `json:"ingredient"` // canonical name or alias
	Tag        *string `json:"tag"`
	Note       *string `json:"note"`
}

var ruleTypes = []string{"allergy", "forbidden", "baby", "health", "taste"}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	var req ruleRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !slices.Contains(ruleTypes, req.Type) {
		writeError(w, http.StatusBadRequest, "type must be one of allergy/forbidden/baby/health/taste")
		return
	}
	if req.Severity == "" {
		req.Severity = "hard"
	}
	if req.Severity != "hard" && req.Severity != "soft" {
		writeError(w, http.StatusBadRequest, "severity must be hard or soft")
		return
	}
	if req.Ingredient == "" && (req.Tag == nil || *req.Tag == "") {
		writeError(w, http.StatusBadRequest, "either ingredient or tag is required")
		return
	}

	var ingredientID *int64
	if req.Ingredient != "" {
		id, err := s.family.ResolveIngredient(r.Context(), req.Ingredient)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "ingredient not found in master data: "+req.Ingredient)
			return
		}
		if err != nil {
			s.fail(w, "resolve ingredient", err)
			return
		}
		ingredientID = &id
	}

	id, err := s.family.CreateRule(r.Context(), claims.HouseholdID, req.MemberID, req.Type, req.Severity, ingredientID, req.Tag, req.Note)
	if err != nil {
		s.fail(w, "create rule", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	err := s.family.DeleteRule(r.Context(), claims.HouseholdID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		s.fail(w, "delete rule", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---- recipe catalog ----

func (s *Server) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	recipes, err := s.family.ListRecipes(r.Context(), q.Get("course"), q.Get("cuisine"), q.Get("q"))
	if err != nil {
		s.fail(w, "list recipes", err)
		return
	}
	writeJSON(w, http.StatusOK, recipes)
}
