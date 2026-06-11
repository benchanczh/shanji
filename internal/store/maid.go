package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaidStore backs the helper-facing surface: magic links, the today
// view and suggestions.
type MaidStore struct {
	pool *pgxpool.Pool
}

func NewMaidStore(pool *pgxpool.Pool) *MaidStore {
	return &MaidStore{pool: pool}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateLink revokes any active link and issues a fresh one. The raw
// token is returned exactly once; only its hash is stored.
func (s *MaidStore) CreateLink(ctx context.Context, householdID int64) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE maid_links SET revoked_at = now()
		WHERE household_id = $1 AND revoked_at IS NULL`, householdID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO maid_links (household_id, token_hash) VALUES ($1, $2)`,
		householdID, hashToken(token)); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}

func (s *MaidStore) RevokeLinks(ctx context.Context, householdID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE maid_links SET revoked_at = now()
		WHERE household_id = $1 AND revoked_at IS NULL`, householdID)
	return err
}

// HouseholdForToken resolves an active magic link to its household.
func (s *MaidStore) HouseholdForToken(ctx context.Context, token string) (int64, error) {
	var householdID int64
	err := s.pool.QueryRow(ctx, `
		SELECT household_id FROM maid_links
		WHERE token_hash = $1 AND revoked_at IS NULL`, hashToken(token)).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return householdID, err
}

// ---- today view ----

type MaidDayView struct {
	Date  string                    `json:"date"`
	Meals map[string][]MaidDishView `json:"meals"`
}

type MaidDishView struct {
	RecipeID    int64                `json:"recipe_id"`
	Name        string               `json:"name"`
	NameEN      string               `json:"name_en"`
	Course      string               `json:"course"`
	Target      string               `json:"target"`
	Minutes     int                  `json:"minutes"`
	Steps       []MaidStepView       `json:"steps"`
	Ingredients []MaidIngredientView `json:"ingredients"`
}

type MaidStepView struct {
	Order          int    `json:"order"`
	TextCN         string `json:"text_cn"`
	TextEN         string `json:"text_en"`
	BabySplitPoint bool   `json:"baby_split_point"`
}

type MaidIngredientView struct {
	Name   string   `json:"name"`
	NameEN string   `json:"name_en"`
	Qty    *float64 `json:"qty"`
	Unit   string   `json:"unit"`
}

// TodayView returns the confirmed plan's meals for one date, with
// bilingual steps and ingredients — everything the helper needs to
// cook without asking.
func (s *MaidStore) TodayView(ctx context.Context, householdID int64, date time.Time) (*MaidDayView, error) {
	var planID int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM weekly_plans
		WHERE household_id = $1 AND status = 'confirmed'
		  AND week_start <= $2 AND week_start > $2::date - 7`, householdID, date).Scan(&planID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ms.meal_type, md.recipe_id, r.name, r.name_en, md.course, md.target, r.minutes
		FROM meal_slots ms
		JOIN meal_dishes md ON md.slot_id = ms.id
		JOIN recipes r ON r.id = md.recipe_id
		WHERE ms.plan_id = $1 AND ms.day = $2
		ORDER BY array_position(ARRAY['breakfast','lunch','dinner'], ms.meal_type), md.id`, planID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	view := &MaidDayView{Date: date.Format("2006-01-02"), Meals: map[string][]MaidDishView{}}
	recipeIDs := map[int64]bool{}
	for rows.Next() {
		var meal string
		var d MaidDishView
		if err := rows.Scan(&meal, &d.RecipeID, &d.Name, &d.NameEN, &d.Course, &d.Target, &d.Minutes); err != nil {
			return nil, err
		}
		view.Meals[meal] = append(view.Meals[meal], d)
		recipeIDs[d.RecipeID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(recipeIDs))
	for id := range recipeIDs {
		ids = append(ids, id)
	}
	steps, ingredients, err := s.recipeDetails(ctx, ids)
	if err != nil {
		return nil, err
	}
	for meal, dishes := range view.Meals {
		for i := range dishes {
			dishes[i].Steps = steps[dishes[i].RecipeID]
			dishes[i].Ingredients = ingredients[dishes[i].RecipeID]
		}
		view.Meals[meal] = dishes
	}
	return view, nil
}

func (s *MaidStore) recipeDetails(ctx context.Context, recipeIDs []int64) (map[int64][]MaidStepView, map[int64][]MaidIngredientView, error) {
	steps := map[int64][]MaidStepView{}
	rows, err := s.pool.Query(ctx, `
		SELECT recipe_id, step_order, text_cn, text_en, baby_split_point
		FROM recipe_steps WHERE recipe_id = ANY($1) ORDER BY recipe_id, step_order`, recipeIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("load steps: %w", err)
	}
	for rows.Next() {
		var id int64
		var st MaidStepView
		if err := rows.Scan(&id, &st.Order, &st.TextCN, &st.TextEN, &st.BabySplitPoint); err != nil {
			rows.Close()
			return nil, nil, err
		}
		steps[id] = append(steps[id], st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	ingredients := map[int64][]MaidIngredientView{}
	rows, err = s.pool.Query(ctx, `
		SELECT ri.recipe_id, i.canonical_name, i.name_en, ri.qty, ri.unit
		FROM recipe_ingredients ri
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ANY($1) ORDER BY ri.recipe_id, ri.id`, recipeIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("load ingredients: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ing MaidIngredientView
		if err := rows.Scan(&id, &ing.Name, &ing.NameEN, &ing.Qty, &ing.Unit); err != nil {
			return nil, nil, err
		}
		ingredients[id] = append(ingredients[id], ing)
	}
	return steps, ingredients, rows.Err()
}

// ---- suggestions ----

type SuggestionView struct {
	ID        int64  `json:"id"`
	FromRole  string `json:"from_role"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (s *MaidStore) CreateSuggestion(ctx context.Context, householdID int64, fromRole, title, content string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO suggestions (household_id, from_role, title, content)
		VALUES ($1, $2, $3, $4) RETURNING id`, householdID, fromRole, title, content).Scan(&id)
	return id, err
}

func (s *MaidStore) ListSuggestions(ctx context.Context, householdID int64, status string) ([]SuggestionView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_role, title, content, status, created_at
		FROM suggestions
		WHERE household_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC`, householdID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SuggestionView{}
	for rows.Next() {
		var v SuggestionView
		var created time.Time
		if err := rows.Scan(&v.ID, &v.FromRole, &v.Title, &v.Content, &v.Status, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = created.Format(time.RFC3339)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetSuggestionStatus performs the two-button review: pending →
// approved/rejected only.
func (s *MaidStore) SetSuggestionStatus(ctx context.Context, householdID, id int64, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE suggestions SET status = $3
		WHERE id = $1 AND household_id = $2 AND status = 'pending'`, id, householdID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
