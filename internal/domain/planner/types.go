// Package planner generates weekly meal plans. It is the core of the
// product and deliberately a pure domain package: no HTTP, no SQL, no
// SDK imports. All inputs arrive as plain values; all randomness is
// injected via a seed so plans are reproducible in tests.
package planner

type Course string

const (
	CourseMain      Course = "main"
	CourseSide      Course = "side"
	CourseSoup      Course = "soup"
	CourseBreakfast Course = "breakfast"
)

type MealType string

const (
	Breakfast MealType = "breakfast"
	Lunch     MealType = "lunch"
	Dinner    MealType = "dinner"
)

type Target string

const (
	TargetAdult Target = "adult"
	TargetBaby  Target = "baby"
)

// Recipe is the planning view of a recipe.
type Recipe struct {
	ID            int64
	Name          string
	Cuisine       string
	Course        Course
	ProteinType   string
	BabyAdaptable bool
	IngredientIDs []int64
	Tags          []string
}

// HardRules are the non-negotiable constraints (allergies, absolute
// dislikes). A recipe touching any banned ingredient or tag is
// excluded before scoring ever happens.
type HardRules struct {
	BannedIngredients map[int64]bool
	BannedTags        map[string]bool
}

// MealSpec is the composition template for lunch/dinner.
type MealSpec struct {
	Mains int
	Sides int
	Soup  bool
}

// Template is the household meal composition for a day.
type Template struct {
	Lunch  MealSpec
	Dinner MealSpec
}

// DefaultTemplate matches the PRD default: 1 荤 + 1 素 + 可选汤.
func DefaultTemplate() Template {
	return Template{
		Lunch:  MealSpec{Mains: 1, Sides: 1, Soup: true},
		Dinner: MealSpec{Mains: 1, Sides: 1, Soup: true},
	}
}

// Config is the household's soft-preference profile.
type Config struct {
	PrimaryCuisine   string
	SecondaryCuisine string
	// CuisineRatio is the target percentage of mains from the
	// primary cuisine across the week (0–100).
	CuisineRatio int
}

// Request is everything the solver needs for one week.
type Request struct {
	Recipes []Recipe
	Rules   HardRules
	Config  Config
	Tmpl    Template
	// HistoryDaysAgo maps recipe ID → days since the household last
	// ate it (counted back from the week start). Used for the
	// 14-day recency penalty.
	HistoryDaysAgo map[int64]int
	Days           int
	BabyMeals      bool
	Seed           int64
}

// Dish is one planned dish inside a meal.
type Dish struct {
	RecipeID int64
	Course   Course
	Target   Target
}

// Slot is one meal of one day.
type Slot struct {
	Day    int // 0-based offset from week start
	Meal   MealType
	Dishes []Dish
}

// WeekPlan is the solver output.
type WeekPlan struct {
	Slots    []Slot
	Warnings []string
}
