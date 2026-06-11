package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/benchanczh/shanji/internal/domain/shopping"
)

// SetSlotLocked toggles a slot lock. Only draft plans owned by the
// household can be modified.
func (s *PlanningStore) SetSlotLocked(ctx context.Context, householdID, slotID int64, locked bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE meal_slots ms SET locked = $3
		FROM weekly_plans wp
		WHERE ms.id = $1 AND wp.id = ms.plan_id
		  AND wp.household_id = $2 AND wp.status = 'draft'`, slotID, householdID, locked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DishContext is everything the swap handler needs to decide a
// replacement and fix the baby linkage.
type DishContext struct {
	DishID      int64
	PlanID      int64
	SlotID      int64
	PlanStatus  string
	Course      string
	Target      string
	RecipeID    int64
	MainIDs     []int64        // all adult main recipe ids in the plan
	SlotDishes  []SlotDishInfo // all dishes in the same slot
}

type SlotDishInfo struct {
	DishID   int64
	RecipeID int64
	Course   string
	Target   string
}

func (s *PlanningStore) GetDishContext(ctx context.Context, householdID, dishID int64) (*DishContext, error) {
	var dc DishContext
	err := s.pool.QueryRow(ctx, `
		SELECT md.id, wp.id, ms.id, wp.status, md.course, md.target, md.recipe_id
		FROM meal_dishes md
		JOIN meal_slots ms ON ms.id = md.slot_id
		JOIN weekly_plans wp ON wp.id = ms.plan_id
		WHERE md.id = $1 AND wp.household_id = $2`, dishID, householdID,
	).Scan(&dc.DishID, &dc.PlanID, &dc.SlotID, &dc.PlanStatus, &dc.Course, &dc.Target, &dc.RecipeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT md.recipe_id FROM meal_dishes md
		JOIN meal_slots ms ON ms.id = md.slot_id
		WHERE ms.plan_id = $1 AND md.course = 'main' AND md.target = 'adult'`, dc.PlanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		dc.MainIDs = append(dc.MainIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	slotRows, err := s.pool.Query(ctx, `
		SELECT id, recipe_id, course, target FROM meal_dishes WHERE slot_id = $1 ORDER BY id`, dc.SlotID)
	if err != nil {
		return nil, err
	}
	defer slotRows.Close()
	for slotRows.Next() {
		var d SlotDishInfo
		if err := slotRows.Scan(&d.DishID, &d.RecipeID, &d.Course, &d.Target); err != nil {
			return nil, err
		}
		dc.SlotDishes = append(dc.SlotDishes, d)
	}
	return &dc, slotRows.Err()
}

// BabyFix describes how the baby dish in the slot must change after a
// swap. Zero value = nothing to do.
type BabyFix struct {
	UpdateDishID int64 // baby dish to repoint (0 = none)
	NewRecipeID  int64
	DeleteDishID int64 // baby dish to remove (0 = none)
}

// ApplySwap atomically repoints the swapped dish and fixes the baby
// linkage.
func (s *PlanningStore) ApplySwap(ctx context.Context, dishID, newRecipeID int64, fix BabyFix) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE meal_dishes SET recipe_id = $2 WHERE id = $1`, dishID, newRecipeID); err != nil {
		return err
	}
	if fix.UpdateDishID != 0 {
		if _, err := tx.Exec(ctx, `UPDATE meal_dishes SET recipe_id = $2 WHERE id = $1`, fix.UpdateDishID, fix.NewRecipeID); err != nil {
			return err
		}
	}
	if fix.DeleteDishID != 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM meal_dishes WHERE id = $1`, fix.DeleteDishID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ConfirmPlan flips a draft to confirmed and materializes its
// shopping list in the same transaction.
func (s *PlanningStore) ConfirmPlan(ctx context.Context, householdID, planID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE weekly_plans SET status = 'confirmed'
		WHERE id = $1 AND household_id = $2 AND status = 'draft'`, planID, householdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	rows, err := tx.Query(ctx, `
		SELECT i.id, i.canonical_name, i.name_en, i.category, ri.qty, ri.unit
		FROM meal_slots ms
		JOIN meal_dishes md ON md.slot_id = ms.id AND md.target = 'adult'
		JOIN recipe_ingredients ri ON ri.recipe_id = md.recipe_id
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ms.plan_id = $1`, planID)
	if err != nil {
		return err
	}
	var lines []shopping.Line
	for rows.Next() {
		var l shopping.Line
		if err := rows.Scan(&l.IngredientID, &l.Name, &l.NameEN, &l.Category, &l.Qty, &l.Unit); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	items := shopping.Aggregate(lines)

	if _, err := tx.Exec(ctx, `DELETE FROM shopping_lists WHERE plan_id = $1`, planID); err != nil {
		return err
	}
	var listID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO shopping_lists (plan_id) VALUES ($1) RETURNING id`, planID).Scan(&listID); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shopping_items (list_id, ingredient_id, name, name_en, total_qty, unit, category)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			listID, it.IngredientID, it.Name, it.NameEN, it.TotalQty, it.Unit, it.Category); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ShoppingListView is the client read model.
type ShoppingListView struct {
	ID    int64              `json:"id"`
	Items []ShoppingItemView `json:"items"`
}

type ShoppingItemView struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	NameEN   string   `json:"name_en"`
	TotalQty *float64 `json:"total_qty"`
	Unit     string   `json:"unit"`
	Category string   `json:"category"`
	Checked  bool     `json:"checked"`
}

func (s *PlanningStore) GetShoppingList(ctx context.Context, householdID, planID int64) (*ShoppingListView, error) {
	var view ShoppingListView
	err := s.pool.QueryRow(ctx, `
		SELECT sl.id FROM shopping_lists sl
		JOIN weekly_plans wp ON wp.id = sl.plan_id
		WHERE sl.plan_id = $1 AND wp.household_id = $2
		ORDER BY sl.version DESC LIMIT 1`, planID, householdID).Scan(&view.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, name_en, total_qty, unit, category, checked
		FROM shopping_items WHERE list_id = $1 ORDER BY id`, view.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it ShoppingItemView
		if err := rows.Scan(&it.ID, &it.Name, &it.NameEN, &it.TotalQty, &it.Unit, &it.Category, &it.Checked); err != nil {
			return nil, err
		}
		view.Items = append(view.Items, it)
	}
	return &view, rows.Err()
}

func (s *PlanningStore) SetItemChecked(ctx context.Context, householdID, itemID int64, checked bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE shopping_items si SET checked = $3
		FROM shopping_lists sl, weekly_plans wp
		WHERE si.id = $1 AND sl.id = si.list_id AND wp.id = sl.plan_id AND wp.household_id = $2`,
		itemID, householdID, checked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("shopping item %d: %w", itemID, ErrNotFound)
	}
	return nil
}
