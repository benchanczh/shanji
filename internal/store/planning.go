package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/domain/planner"
)

// PlanningStore loads solver inputs and persists generated plans.
type PlanningStore struct {
	pool *pgxpool.Pool
}

func NewPlanningStore(pool *pgxpool.Pool) *PlanningStore {
	return &PlanningStore{pool: pool}
}

// LoadActiveRecipes returns all active recipes in planner form,
// including ingredient IDs for hard-rule matching.
func (s *PlanningStore) LoadActiveRecipes(ctx context.Context) ([]planner.Recipe, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.name, r.cuisine, r.course, r.protein_type, r.baby_adaptable, r.nutrition_tags,
		       COALESCE(array_agg(ri.ingredient_id) FILTER (WHERE ri.ingredient_id IS NOT NULL), '{}')
		FROM recipes r
		LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
		WHERE r.status = 'active'
		GROUP BY r.id`)
	if err != nil {
		return nil, fmt.Errorf("load recipes: %w", err)
	}
	defer rows.Close()

	var out []planner.Recipe
	for rows.Next() {
		var r planner.Recipe
		var tagsJSON []byte
		if err := rows.Scan(&r.ID, &r.Name, &r.Cuisine, &r.Course, &r.ProteinType, &r.BabyAdaptable, &tagsJSON, &r.IngredientIDs); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tagsJSON, &r.Tags)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadHardRules returns the household's hard constraints.
func (s *PlanningStore) LoadHardRules(ctx context.Context, householdID int64) (planner.HardRules, error) {
	rules := planner.HardRules{
		BannedIngredients: map[int64]bool{},
		BannedTags:        map[string]bool{},
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ingredient_id, tag FROM diet_rules
		WHERE household_id = $1 AND severity = 'hard'`, householdID)
	if err != nil {
		return rules, fmt.Errorf("load rules: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ingredientID *int64
		var tag *string
		if err := rows.Scan(&ingredientID, &tag); err != nil {
			return rules, err
		}
		if ingredientID != nil {
			rules.BannedIngredients[*ingredientID] = true
		}
		if tag != nil {
			rules.BannedTags[*tag] = true
		}
	}
	return rules, rows.Err()
}

// LoadRecentMains returns recipe ID → days since last eaten, looking
// back over the recency window before weekStart.
func (s *PlanningStore) LoadRecentMains(ctx context.Context, householdID int64, weekStart time.Time) (map[int64]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT md.recipe_id, MAX(ms.day) AS last_day
		FROM meal_dishes md
		JOIN meal_slots ms ON ms.id = md.slot_id
		JOIN weekly_plans wp ON wp.id = ms.plan_id
		WHERE wp.household_id = $1
		  AND md.course = 'main' AND md.target = 'adult'
		  AND ms.day >= $2::date - 14 AND ms.day < $2::date
		GROUP BY md.recipe_id`, householdID, weekStart)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()

	history := map[int64]int{}
	for rows.Next() {
		var recipeID int64
		var lastDay time.Time
		if err := rows.Scan(&recipeID, &lastDay); err != nil {
			return nil, err
		}
		history[recipeID] = int(weekStart.Sub(lastDay).Hours() / 24)
	}
	return history, rows.Err()
}

// SavePlan persists a generated week as a draft, replacing any
// existing draft for the same week. A confirmed plan is never
// overwritten.
func (s *PlanningStore) SavePlan(ctx context.Context, householdID int64, weekStart time.Time, plan planner.WeekPlan) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var existingID int64
	var status string
	err = tx.QueryRow(ctx, `
		SELECT id, status FROM weekly_plans
		WHERE household_id = $1 AND week_start = $2`, householdID, weekStart).Scan(&existingID, &status)
	if err == nil {
		if status == "confirmed" {
			return 0, ErrPlanConfirmed
		}
		if _, err := tx.Exec(ctx, `DELETE FROM weekly_plans WHERE id = $1`, existingID); err != nil {
			return 0, err
		}
	}

	var planID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO weekly_plans (household_id, week_start) VALUES ($1, $2)
		RETURNING id`, householdID, weekStart).Scan(&planID); err != nil {
		return 0, err
	}

	for _, slot := range plan.Slots {
		var slotID int64
		day := weekStart.AddDate(0, 0, slot.Day)
		if err := tx.QueryRow(ctx, `
			INSERT INTO meal_slots (plan_id, day, meal_type, locked) VALUES ($1, $2, $3, $4)
			RETURNING id`, planID, day, slot.Meal, slot.Locked).Scan(&slotID); err != nil {
			return 0, err
		}
		for _, d := range slot.Dishes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO meal_dishes (slot_id, recipe_id, target, course)
				VALUES ($1, $2, $3, $4)`, slotID, d.RecipeID, d.Target, d.Course); err != nil {
				return 0, err
			}
		}
	}
	return planID, tx.Commit(ctx)
}

var ErrPlanConfirmed = fmt.Errorf("plan for this week is already confirmed")

// PlanView is the read model returned to clients.
type PlanView struct {
	ID        int64     `json:"id"`
	WeekStart string    `json:"week_start"`
	Status    string    `json:"status"`
	Days      []DayView `json:"days"`
}

type DayView struct {
	Date  string              `json:"date"`
	Meals map[string]MealView `json:"meals"`
}

type MealView struct {
	SlotID int64      `json:"slot_id"`
	Locked bool       `json:"locked"`
	Dishes []DishView `json:"dishes"`
}

type DishView struct {
	ID       int64  `json:"id"`
	RecipeID int64  `json:"recipe_id"`
	Name     string `json:"name"`
	NameEN   string `json:"name_en"`
	Cuisine  string `json:"cuisine"`
	Course   string `json:"course"`
	Target   string `json:"target"`
}

// GetPlan loads the full plan view for a household week.
func (s *PlanningStore) GetPlan(ctx context.Context, householdID int64, weekStart time.Time) (*PlanView, error) {
	var view PlanView
	var ws time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, week_start, status FROM weekly_plans
		WHERE household_id = $1 AND week_start = $2`, householdID, weekStart).Scan(&view.ID, &ws, &view.Status)
	if err != nil {
		return nil, ErrNotFound
	}
	view.WeekStart = ws.Format("2006-01-02")

	rows, err := s.pool.Query(ctx, `
		SELECT ms.id, ms.locked, ms.day, ms.meal_type,
		       md.id, md.recipe_id, r.name, r.name_en, r.cuisine, md.course, md.target
		FROM meal_slots ms
		JOIN meal_dishes md ON md.slot_id = ms.id
		JOIN recipes r ON r.id = md.recipe_id
		WHERE ms.plan_id = $1
		ORDER BY ms.day, array_position(ARRAY['breakfast','lunch','dinner'], ms.meal_type), md.id`, view.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dayIndex := map[string]int{}
	for rows.Next() {
		var day time.Time
		var meal string
		var slotID int64
		var locked bool
		var d DishView
		if err := rows.Scan(&slotID, &locked, &day, &meal, &d.ID, &d.RecipeID, &d.Name, &d.NameEN, &d.Cuisine, &d.Course, &d.Target); err != nil {
			return nil, err
		}
		key := day.Format("2006-01-02")
		idx, ok := dayIndex[key]
		if !ok {
			idx = len(view.Days)
			dayIndex[key] = idx
			view.Days = append(view.Days, DayView{Date: key, Meals: map[string]MealView{}})
		}
		mv := view.Days[idx].Meals[meal]
		mv.SlotID, mv.Locked = slotID, locked
		mv.Dishes = append(mv.Dishes, d)
		view.Days[idx].Meals[meal] = mv
	}
	return &view, rows.Err()
}

// LoadLockedSlots returns the locked slots of an existing draft for
// the given week, in planner form (day offsets from weekStart).
func (s *PlanningStore) LoadLockedSlots(ctx context.Context, householdID int64, weekStart time.Time) ([]planner.Slot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ms.day, ms.meal_type, md.recipe_id, md.course, md.target
		FROM meal_slots ms
		JOIN weekly_plans wp ON wp.id = ms.plan_id
		JOIN meal_dishes md ON md.slot_id = ms.id
		WHERE wp.household_id = $1 AND wp.week_start = $2 AND wp.status = 'draft' AND ms.locked
		ORDER BY ms.id, md.id`, householdID, weekStart)
	if err != nil {
		return nil, fmt.Errorf("load locked slots: %w", err)
	}
	defer rows.Close()

	byKey := map[string]*planner.Slot{}
	var order []string
	for rows.Next() {
		var day time.Time
		var meal, course, target string
		var recipeID int64
		if err := rows.Scan(&day, &meal, &recipeID, &course, &target); err != nil {
			return nil, err
		}
		offset := int(day.Sub(weekStart).Hours() / 24)
		key := fmt.Sprintf("%d-%s", offset, meal)
		slot, ok := byKey[key]
		if !ok {
			slot = &planner.Slot{Day: offset, Meal: planner.MealType(meal), Locked: true}
			byKey[key] = slot
			order = append(order, key)
		}
		slot.Dishes = append(slot.Dishes, planner.Dish{
			RecipeID: recipeID,
			Course:   planner.Course(course),
			Target:   planner.Target(target),
		})
	}
	var out []planner.Slot
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, rows.Err()
}
