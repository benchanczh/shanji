package planner

import (
	"reflect"
	"testing"
)

func mkMain(id int64, name, cuisine, protein string, baby bool, ingredients ...int64) Recipe {
	return Recipe{ID: id, Name: name, Cuisine: cuisine, Course: CourseMain, ProteinType: protein, BabyAdaptable: baby, IngredientIDs: ingredients}
}

func mkRecipe(id int64, course Course, cuisine string) Recipe {
	return Recipe{ID: id, Name: course2name(course, id), Cuisine: cuisine, Course: course, ProteinType: "none"}
}

func course2name(c Course, id int64) string {
	return string(c) + "-" + string(rune('0'+id%10))
}

// bigPool builds a comfortable pool: 16 mains (8 primary 粤菜,
// 8 家常), 4 sides, 3 soups, 3 breakfasts.
func bigPool() []Recipe {
	proteins := []string{"pork", "chicken", "fish", "beef", "shrimp", "tofu", "egg", "pork"}
	var rs []Recipe
	for i := 0; i < 8; i++ {
		rs = append(rs, mkMain(int64(100+i), "粤main", "粤菜", proteins[i], i%2 == 0, int64(1000+i)))
		rs = append(rs, mkMain(int64(200+i), "家main", "家常", proteins[(i+3)%8], false, int64(2000+i)))
	}
	for i := 0; i < 4; i++ {
		rs = append(rs, mkRecipe(int64(300+i), CourseSide, "家常"))
	}
	for i := 0; i < 3; i++ {
		rs = append(rs, mkRecipe(int64(400+i), CourseSoup, "粤菜"))
	}
	for i := 0; i < 3; i++ {
		rs = append(rs, mkRecipe(int64(500+i), CourseBreakfast, "家常"))
	}
	return rs
}

func baseRequest() Request {
	return Request{
		Recipes: bigPool(),
		Rules:   HardRules{BannedIngredients: map[int64]bool{}, BannedTags: map[string]bool{}},
		Config:  Config{PrimaryCuisine: "粤菜", CuisineRatio: 60},
		Tmpl:    DefaultTemplate(),
		Days:    7,
		Seed:    42,
	}
}

func mainsOf(plan WeekPlan) []int64 {
	var ids []int64
	for _, slot := range plan.Slots {
		for _, d := range slot.Dishes {
			if d.Course == CourseMain && d.Target == TargetAdult {
				ids = append(ids, d.RecipeID)
			}
		}
	}
	return ids
}

func TestHardRulesFilterAllergens(t *testing.T) {
	req := baseRequest()
	req.Rules.BannedIngredients[1000] = true // bans main 100
	req.Rules.BannedIngredients[2003] = true // bans main 203

	plan := Generate(req)
	for _, id := range mainsOf(plan) {
		if id == 100 || id == 203 {
			t.Fatalf("banned recipe %d appeared in plan", id)
		}
	}
}

func TestHardRulesFilterTags(t *testing.T) {
	req := baseRequest()
	spicy := mkMain(999, "辣菜", "川菜", "pork", false, 3000)
	spicy.Tags = []string{"spicy"}
	req.Recipes = append(req.Recipes, spicy)
	req.Rules.BannedTags["spicy"] = true

	for _, id := range mainsOf(Generate(req)) {
		if id == 999 {
			t.Fatal("tag-banned recipe appeared in plan")
		}
	}
}

func TestNoRepeatMainsWhenPoolSufficient(t *testing.T) {
	req := baseRequest() // 16 mains > 14 slots
	plan := Generate(req)

	seen := map[int64]bool{}
	for _, id := range mainsOf(plan) {
		if seen[id] {
			t.Fatalf("main %d repeated within the week despite sufficient pool", id)
		}
		seen[id] = true
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", plan.Warnings)
	}
}

func TestPoolExhaustionRepeatsWithWarning(t *testing.T) {
	req := baseRequest()
	req.Recipes = filter(req.Recipes, func(r Recipe) bool {
		return r.Course != CourseMain || r.ID < 105 // only 5 mains for 14 slots
	})

	plan := Generate(req)
	if got := len(mainsOf(plan)); got != 14 {
		t.Fatalf("expected 14 mains even with small pool, got %d", got)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected pool-exhaustion warning")
	}
}

func TestCuisineRatioGate(t *testing.T) {
	req := baseRequest() // 8 primary mains available, ratio 60% of 14 ≈ 9 → all 8 must be used
	plan := Generate(req)

	primary := 0
	byID := map[int64]Recipe{}
	for _, r := range req.Recipes {
		byID[r.ID] = r
	}
	for _, id := range mainsOf(plan) {
		if byID[id].Cuisine == "粤菜" {
			primary++
		}
	}
	if primary < 8 {
		t.Fatalf("cuisine gate failed: only %d/14 primary mains, want all 8 available used", primary)
	}
}

func TestProteinRotationAvoidsBackToBack(t *testing.T) {
	req := baseRequest()
	plan := Generate(req)

	byID := map[int64]Recipe{}
	for _, r := range req.Recipes {
		byID[r.ID] = r
	}
	ids := mainsOf(plan)
	backToBack := 0
	for i := 1; i < len(ids); i++ {
		a, b := byID[ids[i-1]].ProteinType, byID[ids[i]].ProteinType
		if a == b && a != "none" {
			backToBack++
		}
	}
	// Soft preference: with 7 distinct proteins available, consecutive
	// repeats should be rare, not the norm.
	if backToBack > 3 {
		t.Fatalf("protein rotation too weak: %d back-to-back repeats in %d mains", backToBack, len(ids))
	}
}

func TestRecencyPenaltyPrefersFreshDishes(t *testing.T) {
	// Two identical candidates except one was eaten 3 days ago.
	recent := mkMain(1, "recent", "粤菜", "pork", false, 11)
	fresh := mkMain(2, "fresh", "粤菜", "pork", false, 12)
	req := Request{
		Recipes:        []Recipe{recent, fresh},
		Config:         Config{PrimaryCuisine: "粤菜", CuisineRatio: 60},
		Tmpl:           Template{Lunch: MealSpec{Mains: 1}},
		HistoryDaysAgo: map[int64]int{1: 3},
		Days:           1,
		Seed:           7,
	}
	plan := Generate(req)
	if got := mainsOf(plan); len(got) != 1 || got[0] != 2 {
		t.Fatalf("expected fresh recipe 2 to win over recently-eaten 1, got %v", got)
	}
}

func TestBabyLinksToAdaptableDishInMeal(t *testing.T) {
	req := baseRequest()
	req.BabyMeals = true
	plan := Generate(req)

	adaptable := map[int64]bool{}
	for _, r := range req.Recipes {
		if r.BabyAdaptable {
			adaptable[r.ID] = true
		}
	}
	for _, slot := range plan.Slots {
		if slot.Meal == Breakfast {
			continue
		}
		var babyDish *Dish
		inMeal := map[int64]bool{}
		for i, d := range slot.Dishes {
			if d.Target == TargetBaby {
				babyDish = &slot.Dishes[i]
			} else {
				inMeal[d.RecipeID] = true
			}
		}
		if babyDish == nil {
			t.Fatalf("day %d %s: no baby dish", slot.Day, slot.Meal)
		}
		if !adaptable[babyDish.RecipeID] {
			t.Fatalf("baby dish %d is not baby-adaptable", babyDish.RecipeID)
		}
		// When an adaptable dish is in the meal, the baby dish must
		// link to it (split-before-seasoning) rather than add a new dish.
		hasAdaptableInMeal := false
		for id := range inMeal {
			if adaptable[id] {
				hasAdaptableInMeal = true
			}
		}
		if hasAdaptableInMeal && !inMeal[babyDish.RecipeID] {
			t.Fatalf("day %d %s: baby dish should link to an adaptable dish already in the meal", slot.Day, slot.Meal)
		}
	}
}

func TestBabyWarnsWhenNoOption(t *testing.T) {
	req := baseRequest()
	req.BabyMeals = true
	for i := range req.Recipes {
		req.Recipes[i].BabyAdaptable = false
	}
	plan := Generate(req)
	if len(plan.Warnings) == 0 {
		t.Fatal("expected warning when no baby-adaptable recipes exist")
	}
}

func TestSideGapRotation(t *testing.T) {
	req := baseRequest()
	plan := Generate(req)

	lastDay := map[int64]int{}
	for _, slot := range plan.Slots {
		for _, d := range slot.Dishes {
			if d.Course != CourseSide {
				continue
			}
			if last, ok := lastDay[d.RecipeID]; ok && slot.Day-last < sideGapDays && slot.Day != last {
				t.Fatalf("side %d reused on day %d after day %d (gap < %d)", d.RecipeID, slot.Day, last, sideGapDays)
			}
			lastDay[d.RecipeID] = slot.Day
		}
	}
}

func TestTemplateDrivesMealComposition(t *testing.T) {
	req := baseRequest()
	req.Tmpl = Template{
		Lunch:  MealSpec{Mains: 1, Sides: 0, Soup: false},
		Dinner: MealSpec{Mains: 2, Sides: 1, Soup: true},
	}
	plan := Generate(req)

	for _, slot := range plan.Slots {
		count := map[Course]int{}
		for _, d := range slot.Dishes {
			if d.Target == TargetAdult {
				count[d.Course]++
			}
		}
		switch slot.Meal {
		case Lunch:
			if count[CourseMain] != 1 || count[CourseSide] != 0 || count[CourseSoup] != 0 {
				t.Fatalf("lunch composition wrong: %v", count)
			}
		case Dinner:
			if count[CourseMain] != 2 || count[CourseSide] != 1 || count[CourseSoup] != 1 {
				t.Fatalf("dinner composition wrong: %v", count)
			}
		}
	}
}

func TestBreakfastAvoidsConsecutiveRepeat(t *testing.T) {
	req := baseRequest() // 3 breakfast options
	plan := Generate(req)

	var prev int64
	for _, slot := range plan.Slots {
		if slot.Meal != Breakfast {
			continue
		}
		id := slot.Dishes[0].RecipeID
		if id == prev {
			t.Fatalf("same breakfast %d on consecutive days", id)
		}
		prev = id
	}
}

func TestDeterministicForSameSeed(t *testing.T) {
	a := Generate(baseRequest())
	b := Generate(baseRequest())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed must produce identical plans")
	}
	c := baseRequest()
	c.Seed = 99
	if reflect.DeepEqual(a, Generate(c)) {
		t.Fatal("different seeds should generally produce different plans")
	}
}

func TestLockedRegenerationViaHistoryIsNotNeeded(t *testing.T) {
	// Locked-slot regeneration happens at the API layer by passing
	// already-used mains through usedMainCount preloading — covered
	// when that endpoint lands. This placeholder documents the intent.
	t.Skip("locked-slot regeneration arrives with the swap endpoint")
}
