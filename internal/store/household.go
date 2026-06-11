package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/domain/household"
)

// HouseholdStore persists household profiles.
type HouseholdStore struct {
	pool *pgxpool.Pool
}

func NewHouseholdStore(pool *pgxpool.Pool) *HouseholdStore {
	return &HouseholdStore{pool: pool}
}

func (s *HouseholdStore) Get(ctx context.Context, id int64) (*household.Household, error) {
	var h household.Household
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, primary_cuisine, secondary_cuisine, cuisine_ratio, meal_template, serving_factor
		FROM households WHERE id = $1`, id,
	).Scan(&h.ID, &h.Name, &h.PrimaryCuisine, &h.SecondaryCuisine, &h.CuisineRatio, &h.MealTemplate, &h.ServingFactor)
	if err != nil {
		return nil, fmt.Errorf("get household %d: %w", id, err)
	}
	return &h, nil
}

func (s *HouseholdStore) Update(ctx context.Context, id int64, u household.UpdateProfile) (*household.Household, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE households SET
			name              = COALESCE($2, name),
			primary_cuisine   = COALESCE($3, primary_cuisine),
			secondary_cuisine = COALESCE($4, secondary_cuisine),
			cuisine_ratio     = COALESCE($5, cuisine_ratio),
			meal_template     = COALESCE($6, meal_template),
			serving_factor    = COALESCE($7, serving_factor)
		WHERE id = $1`,
		id, u.Name, u.PrimaryCuisine, u.SecondaryCuisine, u.CuisineRatio, u.MealTemplate, u.ServingFactor,
	)
	if err != nil {
		return nil, fmt.Errorf("update household %d: %w", id, err)
	}
	return s.Get(ctx, id)
}
