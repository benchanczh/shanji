package api

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/benchanczh/shanji/internal/domain/household"
)

func (s *Server) handleGetHousehold(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	h, err := s.households.Get(r.Context(), claims.HouseholdID)
	if err != nil {
		s.log.Error("get household failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleUpdateHousehold(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())

	var update household.UpdateProfile
	if err := decodeBody(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := update.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h, err := s.households.Update(r.Context(), claims.HouseholdID, update)
	if err != nil {
		s.log.Error("update household failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, h)
}
