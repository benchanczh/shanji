// Package household defines the household profile domain types.
// Domain packages must stay free of HTTP, SQL and SDK dependencies.
package household

import "encoding/json"

// Household is the family profile that drives planning: cuisine
// preferences, meal composition template and serving size.
type Household struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	PrimaryCuisine   *string         `json:"primary_cuisine"`
	SecondaryCuisine *string         `json:"secondary_cuisine"`
	CuisineRatio     int             `json:"cuisine_ratio"`
	MealTemplate     json.RawMessage `json:"meal_template"`
	ServingFactor    float64         `json:"serving_factor"`
}

// UpdateProfile is the mutable subset of Household.
type UpdateProfile struct {
	Name             *string          `json:"name"`
	PrimaryCuisine   *string          `json:"primary_cuisine"`
	SecondaryCuisine *string          `json:"secondary_cuisine"`
	CuisineRatio     *int             `json:"cuisine_ratio"`
	MealTemplate     *json.RawMessage `json:"meal_template"`
	ServingFactor    *float64         `json:"serving_factor"`
}

// Validate enforces domain invariants on an update.
func (u UpdateProfile) Validate() error {
	if u.CuisineRatio != nil && (*u.CuisineRatio < 0 || *u.CuisineRatio > 100) {
		return ErrInvalidCuisineRatio
	}
	if u.ServingFactor != nil && (*u.ServingFactor <= 0 || *u.ServingFactor > 20) {
		return ErrInvalidServingFactor
	}
	if u.Name != nil && *u.Name == "" {
		return ErrEmptyName
	}
	return nil
}

type domainError string

func (e domainError) Error() string { return string(e) }

const (
	ErrInvalidCuisineRatio  = domainError("cuisine_ratio must be between 0 and 100")
	ErrInvalidServingFactor = domainError("serving_factor must be between 0 and 20")
	ErrEmptyName            = domainError("name must not be empty")
)
