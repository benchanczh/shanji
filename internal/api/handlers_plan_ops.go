package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/benchanczh/shanji/internal/domain/planner"
	"github.com/benchanczh/shanji/internal/store"
)

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

type lockRequest struct {
	Locked bool `json:"locked"`
}

// PATCH /api/v1/slots/{slotID} — lock or unlock a meal slot.
func (s *Server) handleLockSlot(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	slotID, ok := pathID(r, "slotID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid slot id")
		return
	}
	var req lockRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := s.planning.SetSlotLocked(r.Context(), claims.HouseholdID, slotID, req.Locked)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "slot not found or plan not editable")
		return
	}
	if err != nil {
		s.fail(w, "lock slot", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slot_id": slotID, "locked": req.Locked})
}

// POST /api/v1/dishes/{dishID}/swap — "换一个": replace a dish with a
// fresh candidate of the same course, fixing the baby linkage.
func (s *Server) handleSwapDish(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	dishID, ok := pathID(r, "dishID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid dish id")
		return
	}
	ctx := r.Context()

	dc, err := s.planning.GetDishContext(ctx, claims.HouseholdID, dishID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "dish not found")
		return
	}
	if err != nil {
		s.fail(w, "load dish", err)
		return
	}
	if dc.PlanStatus != "draft" {
		writeError(w, http.StatusConflict, "plan is already confirmed")
		return
	}
	if dc.Target != "adult" {
		writeError(w, http.StatusBadRequest, "baby dishes follow the adult dish; swap the adult dish instead")
		return
	}

	recipes, err := s.planning.LoadActiveRecipes(ctx)
	if err != nil {
		s.fail(w, "load recipes", err)
		return
	}
	rules, err := s.planning.LoadHardRules(ctx, claims.HouseholdID)
	if err != nil {
		s.fail(w, "load rules", err)
		return
	}
	h, err := s.households.Get(ctx, claims.HouseholdID)
	if err != nil {
		s.fail(w, "load household", err)
		return
	}
	cfg := planner.Config{CuisineRatio: h.CuisineRatio}
	if h.PrimaryCuisine != nil {
		cfg.PrimaryCuisine = *h.PrimaryCuisine
	}
	if h.SecondaryCuisine != nil {
		cfg.SecondaryCuisine = *h.SecondaryCuisine
	}

	// Mains must stay unique across the whole week; sides/soups just
	// avoid what is already in this slot.
	exclude := map[int64]bool{dc.RecipeID: true}
	if dc.Course == "main" {
		for _, id := range dc.MainIDs {
			exclude[id] = true
		}
	} else {
		for _, d := range dc.SlotDishes {
			exclude[d.RecipeID] = true
		}
	}

	replacement, found := planner.PickReplacement(
		recipes, rules, cfg, planner.Course(dc.Course), exclude, time.Now().UnixNano())
	if !found {
		writeError(w, http.StatusConflict, "no alternative recipe available for this course")
		return
	}

	fix := babyFixFor(dc, replacement, recipes)
	if err := s.planning.ApplySwap(ctx, dishID, replacement.ID, fix); err != nil {
		s.fail(w, "apply swap", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dish_id":    dishID,
		"recipe_id":  replacement.ID,
		"name":       replacement.Name,
		"cuisine":    replacement.Cuisine,
		"baby_fixed": fix != store.BabyFix{},
	})
}

// babyFixFor recomputes the slot's baby dish after the swapped dish
// changes: keep following the swapped dish if still adaptable,
// otherwise relink to another adaptable dish in the meal, otherwise
// fall back to any baby-friendly recipe, otherwise drop it.
func babyFixFor(dc *store.DishContext, replacement planner.Recipe, recipes []planner.Recipe) store.BabyFix {
	var baby *store.SlotDishInfo
	for i := range dc.SlotDishes {
		if dc.SlotDishes[i].Target == "baby" {
			baby = &dc.SlotDishes[i]
		}
	}
	if baby == nil || baby.RecipeID != dc.RecipeID {
		return store.BabyFix{} // no baby dish, or it follows another dish
	}

	adaptable := map[int64]bool{}
	for _, r := range recipes {
		if r.BabyAdaptable {
			adaptable[r.ID] = true
		}
	}
	if replacement.BabyAdaptable {
		return store.BabyFix{UpdateDishID: baby.DishID, NewRecipeID: replacement.ID}
	}
	for _, d := range dc.SlotDishes {
		if d.Target == "adult" && d.DishID != dc.DishID && adaptable[d.RecipeID] {
			return store.BabyFix{UpdateDishID: baby.DishID, NewRecipeID: d.RecipeID}
		}
	}
	for _, r := range recipes {
		if r.BabyAdaptable {
			return store.BabyFix{UpdateDishID: baby.DishID, NewRecipeID: r.ID}
		}
	}
	return store.BabyFix{DeleteDishID: baby.DishID}
}

// POST /api/v1/plans/{planID}/confirm — freeze the week and build the
// shopping list.
func (s *Server) handleConfirmPlan(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	planID, ok := pathID(r, "planID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	err := s.planning.ConfirmPlan(r.Context(), claims.HouseholdID, planID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "plan not found or already confirmed")
		return
	}
	if err != nil {
		s.fail(w, "confirm plan", err)
		return
	}
	list, err := s.planning.GetShoppingList(r.Context(), claims.HouseholdID, planID)
	if err != nil {
		s.fail(w, "load shopping list", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan_id": planID, "status": "confirmed", "shopping_list": list})
}

// GET /api/v1/plans/{planID}/shopping-list
func (s *Server) handleGetShoppingList(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	planID, ok := pathID(r, "planID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	list, err := s.planning.GetShoppingList(r.Context(), claims.HouseholdID, planID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no shopping list; confirm the plan first")
		return
	}
	if err != nil {
		s.fail(w, "load shopping list", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type checkItemRequest struct {
	Checked bool `json:"checked"`
}

// PATCH /api/v1/shopping-items/{itemID}
func (s *Server) handleCheckItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFrom(r.Context())
	itemID, ok := pathID(r, "itemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	var req checkItemRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := s.planning.SetItemChecked(r.Context(), claims.HouseholdID, itemID, req.Checked)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		s.fail(w, "check item", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item_id": itemID, "checked": req.Checked})
}
