// Command seed imports recipe library content from a JSON file.
// The same JSON contract will be produced by the AI generation
// pipeline later; every ingredient must resolve against the
// ingredients master data (or be defined in the same file) — recipes
// with unresolved ingredients are rejected, never silently inserted.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/config"
)

type seedFile struct {
	Ingredients []seedIngredient `json:"ingredients"`
	Recipes     []seedRecipe     `json:"recipes"`
}

type seedIngredient struct {
	CanonicalName string   `json:"canonical_name"`
	NameEN        string   `json:"name_en"`
	Category      string   `json:"category"`
	DefaultUnit   string   `json:"default_unit"`
	Aliases       []string `json:"aliases"`
}

type seedRecipe struct {
	Name          string             `json:"name"`
	NameEN        string             `json:"name_en"`
	Cuisine       string             `json:"cuisine"`
	Course        string             `json:"course"`
	Minutes       int                `json:"minutes"`
	Difficulty    string             `json:"difficulty"`
	ProteinType   string             `json:"protein_type"`
	NutritionTags []string           `json:"nutrition_tags"`
	BabyAdaptable bool               `json:"baby_adaptable"`
	Ingredients   []seedRecipeIngred `json:"ingredients"`
	Steps         []seedStep         `json:"steps"`
}

type seedRecipeIngred struct {
	Name string   `json:"name"`
	Qty  *float64 `json:"qty"` // nil = 适量, excluded from shopping aggregation
	Unit string   `json:"unit"`
	Note string   `json:"note,omitempty"`
}

type seedStep struct {
	CN             string `json:"cn"`
	EN             string `json:"en"`
	BabySplitPoint bool   `json:"baby_split_point,omitempty"`
}

func main() {
	path := flag.String("file", "seed/seed.json", "seed JSON file")
	flag.Parse()

	if err := run(*path); err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
}

func run(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var sf seedFile
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sf); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validate(sf); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ingredientIDs, err := upsertIngredients(ctx, tx, sf.Ingredients)
	if err != nil {
		return err
	}

	inserted, skipped := 0, 0
	for _, r := range sf.Recipes {
		ok, err := insertRecipe(ctx, tx, r, ingredientIDs)
		if err != nil {
			return fmt.Errorf("recipe %q: %w", r.Name, err)
		}
		if ok {
			inserted++
		} else {
			skipped++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("ingredients upserted: %d, recipes inserted: %d, skipped (already exist): %d\n",
		len(sf.Ingredients), inserted, skipped)
	return nil
}

// validate enforces the E3 discipline before touching the database:
// every recipe ingredient must resolve to a canonical name or alias
// defined in this file.
func validate(sf seedFile) error {
	known := map[string]bool{}
	for _, ing := range sf.Ingredients {
		if ing.CanonicalName == "" {
			return fmt.Errorf("ingredient with empty canonical_name")
		}
		known[ing.CanonicalName] = true
		for _, a := range ing.Aliases {
			known[a] = true
		}
	}
	for _, r := range sf.Recipes {
		if len(r.Steps) < 2 {
			return fmt.Errorf("recipe %q: needs at least 2 steps", r.Name)
		}
		if len(r.Ingredients) == 0 {
			return fmt.Errorf("recipe %q: no ingredients", r.Name)
		}
		for _, ri := range r.Ingredients {
			if !known[ri.Name] {
				return fmt.Errorf("recipe %q: ingredient %q not in master data (E3 violation)", r.Name, ri.Name)
			}
		}
	}
	return nil
}

func upsertIngredients(ctx context.Context, tx pgx.Tx, ings []seedIngredient) (map[string]int64, error) {
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

func insertRecipe(ctx context.Context, tx pgx.Tx, r seedRecipe, ingredientIDs map[string]int64) (bool, error) {
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
		VALUES ($1, $2, $3, $4, 'library', 'active', $5, $6, $7, $8, $9)
		RETURNING id`,
		r.Name, r.NameEN, r.Cuisine, r.Course, r.Minutes, r.Difficulty, r.ProteinType, tags, r.BabyAdaptable,
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
