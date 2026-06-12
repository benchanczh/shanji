package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/seedjson"
)

// SeedStore imports library content (manual seeds and AI output)
// through one code path.
type SeedStore struct {
	pool *pgxpool.Pool
}

func NewSeedStore(pool *pgxpool.Pool) *SeedStore {
	return &SeedStore{pool: pool}
}

// KnownIngredientNames returns the current vocabulary: canonical
// names and aliases.
func (s *SeedStore) KnownIngredientNames(ctx context.Context) (map[string]bool, error) {
	known := map[string]bool{}
	rows, err := s.pool.Query(ctx, `
		SELECT canonical_name FROM ingredients
		UNION ALL
		SELECT alias FROM ingredient_aliases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		known[name] = true
	}
	return known, rows.Err()
}

// RecipeNamesByGroup returns existing recipe names (any status)
// grouped by cuisine+course, for dedup and deficit planning.
func (s *SeedStore) RecipeNamesByGroup(ctx context.Context) (map[[2]string][]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cuisine, course, name FROM recipes WHERE status IN ('active', 'pending_review')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[[2]string][]string{}
	for rows.Next() {
		var cuisine, course, name string
		if err := rows.Scan(&cuisine, &course, &name); err != nil {
			return nil, err
		}
		key := [2]string{cuisine, course}
		out[key] = append(out[key], name)
	}
	return out, rows.Err()
}

// Import inserts a validated seed file. Existing recipes (by name)
// are skipped; ingredients are upserted. source/status apply to all
// inserted recipes ('library'/'active' for curated seeds,
// 'ai'/'pending_review' for generated ones).
func (s *SeedStore) Import(ctx context.Context, f *seedjson.File, source, status string) (inserted, skipped int, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	ids, err := s.upsertIngredients(ctx, tx, f.Ingredients)
	if err != nil {
		return 0, 0, err
	}
	// Resolve against pre-existing vocabulary too.
	rows, err := tx.Query(ctx, `
		SELECT canonical_name, id FROM ingredients
		UNION ALL
		SELECT a.alias, a.ingredient_id FROM ingredient_aliases a`)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if _, ok := ids[name]; !ok {
			ids[name] = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, r := range f.Recipes {
		ok, err := s.insertRecipe(ctx, tx, r, ids, source, status)
		if err != nil {
			return 0, 0, fmt.Errorf("recipe %q: %w", r.Name, err)
		}
		if ok {
			inserted++
		} else {
			skipped++
		}
	}
	return inserted, skipped, tx.Commit(ctx)
}

func (s *SeedStore) upsertIngredients(ctx context.Context, tx pgx.Tx, ings []seedjson.Ingredient) (map[string]int64, error) {
	ids := map[string]int64{}
	for _, ing := range ings {
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO ingredients (canonical_name, name_en, category, default_unit)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (canonical_name) DO UPDATE
				SET name_en = EXCLUDED.name_en, category = EXCLUDED.category, default_unit = EXCLUDED.default_unit
			RETURNING id`,
			ing.CanonicalName, ing.NameEN, ing.Category, ing.DefaultUnit,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert ingredient %q: %w", ing.CanonicalName, err)
		}
		ids[ing.CanonicalName] = id
		for _, alias := range ing.Aliases {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ingredient_aliases (ingredient_id, alias)
				VALUES ($1, $2) ON CONFLICT (alias) DO NOTHING`, id, alias); err != nil {
				return nil, fmt.Errorf("alias %q: %w", alias, err)
			}
			ids[alias] = id
		}
	}
	return ids, nil
}

func (s *SeedStore) insertRecipe(ctx context.Context, tx pgx.Tx, r seedjson.Recipe, ingredientIDs map[string]int64, source, status string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM recipes WHERE name = $1)`, r.Name).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	tags, _ := json.Marshal(r.NutritionTags)
	var recipeID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO recipes (name, name_en, cuisine, course, source, status, minutes, difficulty, protein_type, nutrition_tags, baby_adaptable)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		r.Name, r.NameEN, r.Cuisine, r.Course, source, status, r.Minutes, r.Difficulty, r.ProteinType, tags, r.BabyAdaptable,
	).Scan(&recipeID)
	if err != nil {
		return false, err
	}

	for _, ri := range r.Ingredients {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_ingredients (recipe_id, ingredient_id, qty, unit, note)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''))`,
			recipeID, ingredientIDs[ri.Name], ri.Qty, ri.Unit, ri.Note); err != nil {
			return false, err
		}
	}
	for i, st := range r.Steps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_steps (recipe_id, step_order, text_cn, text_en, baby_split_point)
			VALUES ($1, $2, $3, $4, $5)`,
			recipeID, i+1, st.CN, st.EN, st.BabySplitPoint); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ActivatePendingAI flips reviewed AI recipes to active. Called after
// a human spot-check of pending_review content.
func (s *SeedStore) ActivatePendingAI(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE recipes SET status = 'active' WHERE status = 'pending_review' AND source = 'ai'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ListPendingAI returns pending AI recipes for review.
func (s *SeedStore) ListPendingAI(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cuisine || '/' || course || ' ' || name || ' (' || name_en || ')'
		FROM recipes WHERE status = 'pending_review' AND source = 'ai' ORDER BY cuisine, course, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
