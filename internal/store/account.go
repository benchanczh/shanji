package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account is an authenticated household login (decider/spouse).
type Account struct {
	ID           int64
	HouseholdID  int64
	Username     string
	PasswordHash string
	Role         string
}

var ErrNotFound = errors.New("not found")

type AccountStore struct {
	pool *pgxpool.Pool
}

func NewAccountStore(pool *pgxpool.Pool) *AccountStore {
	return &AccountStore{pool: pool}
}

func (s *AccountStore) GetByUsername(ctx context.Context, username string) (*Account, error) {
	var a Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, household_id, username, password_hash, role
		FROM accounts WHERE username = $1`, username,
	).Scan(&a.ID, &a.HouseholdID, &a.Username, &a.PasswordHash, &a.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account %q: %w", username, err)
	}
	return &a, nil
}
