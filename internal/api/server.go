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
}

func NewServer(cfg *config.Config, log *zap.Logger, pool *pgxpool.Pool) *Server {
	return &Server{
		cfg:        cfg,
		log:        log,
		accounts:   store.NewAccountStore(pool),
		households: store.NewHouseholdStore(pool),
		planning:   store.NewPlanningStore(pool),
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

	return s.withMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
