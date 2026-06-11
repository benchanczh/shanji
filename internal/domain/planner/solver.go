package planner

import (
	"fmt"
	"math/rand"
	"slices"
)

// Scoring weights. Order of precedence mirrors the PRD: hard rules are
// a filter (not a score), then no-repeat, then cuisine preference,
// protein rotation and recency.
const (
	scorePrimaryCuisine   = 30
	scoreSecondaryCuisine = 15
	scoreProteinRotation  = 20
	penaltyRecency        = 25 // eaten within the last 14 days
	penaltyReuse          = 40 // per prior use within this week (pool exhausted)
	jitter                = 5  // random tie-break
	recencyWindowDays     = 14
	sideGapDays           = 2 // same side/soup allowed again after this many days
)

// Generate solves one week. It is deterministic for a given Request
// (including Seed).
func Generate(req Request) WeekPlan {
	if req.Days <= 0 {
		req.Days = 7
	}
	s := newState(req)
	plan := WeekPlan{}

	locked := map[[2]any]Slot{}
	for _, slot := range req.Locked {
		locked[[2]any{slot.Day, slot.Meal}] = slot
	}
	build := func(day int, meal MealType, fill func() Slot) Slot {
		if slot, ok := locked[[2]any{day, meal}]; ok {
			slot.Locked = true
			s.adoptLocked(slot)
			return slot
		}
		return fill()
	}

	for day := 0; day < req.Days; day++ {
		plan.Slots = append(plan.Slots,
			build(day, Breakfast, func() Slot { return s.buildBreakfast(day) }),
			build(day, Lunch, func() Slot { return s.buildMeal(day, Lunch, req.Tmpl.Lunch) }),
			build(day, Dinner, func() Slot { return s.buildMeal(day, Dinner, req.Tmpl.Dinner) }),
		)
	}
	plan.Warnings = s.warnings
	return plan
}

// adoptLocked registers a kept slot's dishes in the rotation state so
// the rest of the week plans around them.
func (s *state) adoptLocked(slot Slot) {
	byID := map[int64]Recipe{}
	for _, r := range s.req.Recipes {
		byID[r.ID] = r
	}
	for _, d := range slot.Dishes {
		if d.Target != TargetAdult {
			continue
		}
		r, known := byID[d.RecipeID]
		switch d.Course {
		case CourseMain:
			s.usedMainCount[d.RecipeID]++
			s.mainsPicked++
			if known {
				if r.Cuisine == s.req.Config.PrimaryCuisine {
					s.primaryPicked++
				}
				if r.ProteinType != "none" {
					s.prevProtein = r.ProteinType
				}
			}
		case CourseSide:
			s.sideLastDay[d.RecipeID] = slot.Day
		case CourseSoup:
			s.soupLastDay[d.RecipeID] = slot.Day
		case CourseBreakfast:
			s.breakfastCount[d.RecipeID]++
			s.lastBreakfast = d.RecipeID
		}
	}
}

// PickReplacement chooses a new recipe for "换一个": same course,
// passing hard rules, not in the exclusion set, preferring the
// household cuisine. Returns false when nothing qualifies.
func PickReplacement(recipes []Recipe, rules HardRules, cfg Config, course Course, exclude map[int64]bool, seed int64) (Recipe, bool) {
	pool := filter(recipes, func(r Recipe) bool {
		return r.Course == course && !exclude[r.ID] && passesHardRules(r, rules)
	})
	if len(pool) == 0 {
		return Recipe{}, false
	}
	rng := rand.New(rand.NewSource(seed))
	best := pool[0]
	bestScore := -1 << 30
	for _, r := range pool {
		score := rng.Intn(jitter)
		switch r.Cuisine {
		case cfg.PrimaryCuisine:
			score += scorePrimaryCuisine
		case cfg.SecondaryCuisine:
			score += scoreSecondaryCuisine
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	return best, true
}

type state struct {
	req Request
	rng *rand.Rand

	mains, sides, soups, breakfasts, babyPool []Recipe

	usedMainCount  map[int64]int // uses this week (>0 means repeat)
	mainsPicked    int
	primaryPicked  int
	prevProtein    string
	sideLastDay    map[int64]int
	soupLastDay    map[int64]int
	breakfastCount map[int64]int
	lastBreakfast  int64
	babyUseCount   map[int64]int

	warnings []string
}

func newState(req Request) *state {
	s := &state{
		req:            req,
		rng:            rand.New(rand.NewSource(req.Seed)),
		usedMainCount:  map[int64]int{},
		sideLastDay:    map[int64]int{},
		soupLastDay:    map[int64]int{},
		breakfastCount: map[int64]int{},
		babyUseCount:   map[int64]int{},
	}
	for _, r := range req.Recipes {
		if !passesHardRules(r, req.Rules) {
			continue
		}
		switch r.Course {
		case CourseMain:
			s.mains = append(s.mains, r)
		case CourseSide:
			s.sides = append(s.sides, r)
		case CourseSoup:
			s.soups = append(s.soups, r)
		case CourseBreakfast:
			s.breakfasts = append(s.breakfasts, r)
		}
		if r.BabyAdaptable {
			s.babyPool = append(s.babyPool, r)
		}
	}
	return s
}

func passesHardRules(r Recipe, rules HardRules) bool {
	for _, id := range r.IngredientIDs {
		if rules.BannedIngredients[id] {
			return false
		}
	}
	for _, t := range r.Tags {
		if rules.BannedTags[t] {
			return false
		}
	}
	return true
}

func (s *state) buildBreakfast(day int) Slot {
	slot := Slot{Day: day, Meal: Breakfast}
	if len(s.breakfasts) == 0 {
		s.warnf("no breakfast recipes available")
		return slot
	}
	best := s.pickBest(s.breakfasts, func(r Recipe) int {
		score := -10 * s.breakfastCount[r.ID] // rotate through the small set
		if r.ID == s.lastBreakfast {
			score -= 15 // avoid the same breakfast two days in a row
		}
		return score
	})
	s.breakfastCount[best.ID]++
	s.lastBreakfast = best.ID
	slot.Dishes = append(slot.Dishes, Dish{RecipeID: best.ID, Course: CourseBreakfast, Target: TargetAdult})
	return slot
}

func (s *state) buildMeal(day int, meal MealType, spec MealSpec) Slot {
	slot := Slot{Day: day, Meal: meal}

	for i := 0; i < spec.Mains; i++ {
		if main, ok := s.pickMain(day, meal); ok {
			slot.Dishes = append(slot.Dishes, Dish{RecipeID: main.ID, Course: CourseMain, Target: TargetAdult})
		}
	}
	for i := 0; i < spec.Sides; i++ {
		if side, ok := s.pickWithGap(s.sides, s.sideLastDay, day); ok {
			slot.Dishes = append(slot.Dishes, Dish{RecipeID: side.ID, Course: CourseSide, Target: TargetAdult})
		}
	}
	if spec.Soup {
		if soup, ok := s.pickWithGap(s.soups, s.soupLastDay, day); ok {
			slot.Dishes = append(slot.Dishes, Dish{RecipeID: soup.ID, Course: CourseSoup, Target: TargetAdult})
		}
	}

	if s.req.BabyMeals {
		if baby, ok := s.pickBaby(slot); ok {
			slot.Dishes = append(slot.Dishes, baby)
		} else {
			s.warnf("no baby-adaptable option for day %d %s", day+1, meal)
		}
	}
	return slot
}

// pickMain selects the 荤 main. The cuisine-ratio gate runs first:
// while the week's primary-cuisine share is below target and unused
// primary mains remain, only primary-cuisine candidates compete.
func (s *state) pickMain(day int, meal MealType) (Recipe, bool) {
	if len(s.mains) == 0 {
		s.warnf("no main recipes available for day %d %s", day+1, meal)
		return Recipe{}, false
	}

	unused := filter(s.mains, func(r Recipe) bool { return s.usedMainCount[r.ID] == 0 })
	candidates := unused
	relaxed := false
	if len(candidates) == 0 {
		candidates = s.mains // pool exhausted: repeats allowed, penalized per use
		relaxed = true
	}

	if gated := s.applyCuisineGate(candidates); len(gated) > 0 {
		candidates = gated
	}

	best := s.pickBest(candidates, func(r Recipe) int {
		score := 0
		switch r.Cuisine {
		case s.req.Config.PrimaryCuisine:
			score += scorePrimaryCuisine
		case s.req.Config.SecondaryCuisine:
			score += scoreSecondaryCuisine
		}
		if s.prevProtein == "" || (r.ProteinType != s.prevProtein && r.ProteinType != "none") {
			score += scoreProteinRotation
		}
		if daysAgo, ok := s.req.HistoryDaysAgo[r.ID]; ok && daysAgo <= recencyWindowDays {
			score -= penaltyRecency
		}
		score -= penaltyReuse * s.usedMainCount[r.ID]
		return score
	})

	if relaxed {
		s.warnf("main pool exhausted on day %d %s: repeating %s", day+1, meal, best.Name)
	}
	s.usedMainCount[best.ID]++
	s.mainsPicked++
	if best.Cuisine == s.req.Config.PrimaryCuisine {
		s.primaryPicked++
	}
	if best.ProteinType != "none" {
		s.prevProtein = best.ProteinType
	}
	return best, true
}

// applyCuisineGate returns only primary-cuisine candidates when the
// week is running below the household's target share. Empty result
// means the gate cannot help (no primary candidates left).
func (s *state) applyCuisineGate(candidates []Recipe) []Recipe {
	cfg := s.req.Config
	if cfg.PrimaryCuisine == "" || cfg.CuisineRatio <= 0 {
		return nil
	}
	// Share if we pick a non-primary main now.
	projected := (s.primaryPicked * 100) / (s.mainsPicked + 1)
	if projected >= cfg.CuisineRatio {
		return nil
	}
	return filter(candidates, func(r Recipe) bool { return r.Cuisine == cfg.PrimaryCuisine })
}

func (s *state) pickWithGap(pool []Recipe, lastDay map[int64]int, day int) (Recipe, bool) {
	if len(pool) == 0 {
		return Recipe{}, false
	}
	fresh := filter(pool, func(r Recipe) bool {
		last, used := lastDay[r.ID]
		return !used || day-last >= sideGapDays
	})
	candidates := fresh
	if len(candidates) == 0 {
		candidates = pool
	}
	best := s.pickBest(candidates, func(r Recipe) int {
		score := 0
		if r.Cuisine == s.req.Config.PrimaryCuisine {
			score += scoreSecondaryCuisine // mild cuisine pull for sides/soups
		}
		if last, used := lastDay[r.ID]; used {
			score -= (sideGapDays - min(day-last, sideGapDays)) * 10
		}
		return score
	})
	lastDay[best.ID] = day
	return best, true
}

// pickBaby links the baby meal to an adaptable dish already in the
// slot (split-before-seasoning), falling back to the global
// baby-friendly pool as an independent simple dish.
func (s *state) pickBaby(slot Slot) (Dish, bool) {
	adaptable := map[int64]bool{}
	for _, r := range s.babyPool {
		adaptable[r.ID] = true
	}
	for _, d := range slot.Dishes {
		if adaptable[d.RecipeID] {
			return Dish{RecipeID: d.RecipeID, Course: d.Course, Target: TargetBaby}, true
		}
	}
	if len(s.babyPool) == 0 {
		return Dish{}, false
	}
	best := s.pickBest(s.babyPool, func(r Recipe) int {
		return -10 * s.babyUseCount[r.ID]
	})
	s.babyUseCount[best.ID]++
	return Dish{RecipeID: best.ID, Course: best.Course, Target: TargetBaby}, true
}

// pickBest scores all candidates (plus deterministic jitter) and
// returns the highest. Ties resolve by jitter, making plans varied
// across seeds but reproducible for one seed.
func (s *state) pickBest(candidates []Recipe, score func(Recipe) int) Recipe {
	best := candidates[0]
	bestScore := score(best) + s.rng.Intn(jitter)
	for _, r := range candidates[1:] {
		if sc := score(r) + s.rng.Intn(jitter); sc > bestScore {
			best, bestScore = r, sc
		}
	}
	return best
}

func (s *state) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if !slices.Contains(s.warnings, msg) {
		s.warnings = append(s.warnings, msg)
	}
}

func filter(rs []Recipe, keep func(Recipe) bool) []Recipe {
	var out []Recipe
	for _, r := range rs {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}
