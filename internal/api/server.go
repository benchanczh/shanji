// Package api is the thin HTTP adapter layer: routing, auth, DTO
// mapping. Business logic lives in internal/domain.
package api

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/benchanczh/shanji/internal/config"
	"github.com/benchanczh/shanji/internal/store"
)

type Server struct {
	cfg        *config.Config
	log        *zap.Logger
	accounts   *store.AccountStore
	households *store.HouseholdStore
	planning   *store.PlanningStore
	maid       *store.MaidStore
	family     *store.FamilyStore
}

func NewServer(cfg *config.Config, log *zap.Logger, pool *pgxpool.Pool) *Server {
	return &Server{
		cfg:        cfg,
		log:        log,
		accounts:   store.NewAccountStore(pool),
		households: store.NewHouseholdStore(pool),
		planning:   store.NewPlanningStore(pool),
		maid:       store.NewMaidStore(pool),
		family:     store.NewFamilyStore(pool),
	}
}

// Handler builds the full route table under /api/v1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/v1/household", s.requireAuth(s.handleGetHousehold))
	mux.HandleFunc("PUT /api/v1/household", s.requireAuth(s.handleUpdateHousehold))
	mux.HandleFunc("POST /api/v1/plans/generate", s.requireAuth(s.handleGeneratePlan))
	mux.HandleFunc("GET /api/v1/plans", s.requireAuth(s.handleGetPlan))
	mux.HandleFunc("PATCH /api/v1/slots/{slotID}", s.requireAuth(s.handleLockSlot))
	mux.HandleFunc("POST /api/v1/dishes/{dishID}/swap", s.requireAuth(s.handleSwapDish))
	mux.HandleFunc("POST /api/v1/plans/{planID}/confirm", s.requireAuth(s.handleConfirmPlan))
	mux.HandleFunc("GET /api/v1/plans/{planID}/shopping-list", s.requireAuth(s.handleGetShoppingList))
	mux.HandleFunc("PATCH /api/v1/shopping-items/{itemID}", s.requireAuth(s.handleCheckItem))

	mux.HandleFunc("GET /api/v1/members", s.requireAuth(s.handleListMembers))
	mux.HandleFunc("POST /api/v1/members", s.requireAuth(s.handleCreateMember))
	mux.HandleFunc("DELETE /api/v1/members/{id}", s.requireAuth(s.handleDeleteMember))
	mux.HandleFunc("GET /api/v1/rules", s.requireAuth(s.handleListRules))
	mux.HandleFunc("POST /api/v1/rules", s.requireAuth(s.handleCreateRule))
	mux.HandleFunc("DELETE /api/v1/rules/{id}", s.requireAuth(s.handleDeleteRule))
	mux.HandleFunc("GET /api/v1/recipes", s.requireAuth(s.handleListRecipes))

	mux.HandleFunc("POST /api/v1/maid-link", s.requireAuth(s.handleCreateMaidLink))
	mux.HandleFunc("DELETE /api/v1/maid-link", s.requireAuth(s.handleRevokeMaidLink))
	mux.HandleFunc("GET /api/v1/maid/today", s.requireMaidToken(s.handleMaidToday))
	mux.HandleFunc("POST /api/v1/maid/suggestions", s.requireMaidToken(s.handleMaidSuggest))
	mux.HandleFunc("GET /api/v1/suggestions", s.requireAuth(s.handleListSuggestions))
	mux.HandleFunc("POST /api/v1/suggestions/{id}/approve", s.requireAuth(s.handleReviewSuggestion("approved")))
	mux.HandleFunc("POST /api/v1/suggestions/{id}/reject", s.requireAuth(s.handleReviewSuggestion("rejected")))

	return s.withMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
