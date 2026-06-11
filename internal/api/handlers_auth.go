package api

import (
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/benchanczh/shanji/internal/auth"
	"github.com/benchanczh/shanji/internal/store"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token   string      `json:"token"`
	Account accountInfo `json:"account"`
}

type accountInfo struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	HouseholdID int64  `json:"household_id"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	account, err := s.accounts.GetByUsername(r.Context(), req.Username)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !auth.CheckPassword(account.PasswordHash, req.Password)) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		s.log.Error("login lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	token, err := auth.IssueToken(s.cfg.JWTSecret, account.ID, account.HouseholdID, account.Role)
	if err != nil {
		s.log.Error("issue token failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token: token,
		Account: accountInfo{
			ID:          account.ID,
			Username:    account.Username,
			Role:        account.Role,
			HouseholdID: account.HouseholdID,
		},
	})
}
