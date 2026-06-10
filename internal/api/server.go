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
}

func NewServer(cfg *config.Config, log *zap.Logger, pool *pgxpool.Pool) *Server {
	return &Server{
		cfg:        cfg,
		log:        log,
		accounts:   store.NewAccountStore(pool),
		households: store.NewHouseholdStore(pool),
	}
}

// Handler builds the full route table under /api/v1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/v1/household", s.requireAuth(s.handleGetHousehold))
	mux.HandleFunc("PUT /api/v1/household", s.requireAuth(s.handleUpdateHousehold))

	return s.withMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
