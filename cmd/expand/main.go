// Command expand grows the recipe library with AI-generated recipes
// until each cuisine/course group reaches its target depth (T3: every
// cuisine needs enough mains to fill two weeks without repeats).
//
// Generated recipes enter as status=pending_review, source=ai. After
// a human spot-check, run with -activate to flip them active.
//
// Usage:
//
//	go run ./cmd/expand -dry          # show the deficit plan, no API calls
//	go run ./cmd/expand -max 20       # generate up to 20 recipes
//	go run ./cmd/expand -review       # list pending AI recipes
//	go run ./cmd/expand -activate     # activate all pending AI recipes
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/ai"
	"github.com/benchanczh/shanji/internal/config"
	"github.com/benchanczh/shanji/internal/store"
)

// targets defines the desired library depth per cuisine+course.
// Mains: ≥14 per supported cuisine covers the 14-day no-repeat
// window; sides/soups/breakfast rotate, so smaller pools suffice.
var targets = []struct {
	cuisine, course string
	want            int
}{
	{"粤菜", "main", 14},
	{"川菜", "main", 10},
	{"湘菜", "main", 8},
	{"江浙菜", "main", 10},
	{"家常", "main", 10},
	{"家常", "side", 10},
	{"粤菜", "side", 4},
	{"粤菜", "soup", 5},
	{"家常", "soup", 4},
	{"家常", "breakfast", 4},
	{"粤菜", "breakfast", 3},
}

func main() {
	dry := flag.Bool("dry", false, "print the deficit plan without calling the API")
	maxN := flag.Int("max", 20, "maximum recipes to generate in one run")
	review := flag.Bool("review", false, "list pending AI recipes and exit")
	activate := flag.Bool("activate", false, "activate all pending AI recipes and exit")
	flag.Parse()

	if err := run(*dry, *maxN, *review, *activate); err != nil {
		fmt.Fprintln(os.Stderr, "expand failed:", err)
		os.Exit(1)
	}
}

func run(dry bool, maxN int, review, activate bool) error {
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
	seeds := store.NewSeedStore(pool)

	if review {
		pending, err := seeds.ListPendingAI(ctx)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			fmt.Println("no pending AI recipes")
			return nil
		}
		for _, p := range pending {
			fmt.Println(" -", p)
		}
		fmt.Printf("%d pending — spot-check in the app or DB, then run -activate\n", len(pending))
		return nil
	}
	if activate {
		n, err := seeds.ActivatePendingAI(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("activated %d AI recipes\n", n)
		return nil
	}

	existing, err := seeds.RecipeNamesByGroup(ctx)
	if err != nil {
		return err
	}
	known, err := seeds.KnownIngredientNames(ctx)
	if err != nil {
		return err
	}
	knownList := make([]string, 0, len(known))
	for name := range known {
		knownList = append(knownList, name)
	}
	sort.Strings(knownList)

	type job struct{ cuisine, course string }
	var jobs []job
	for _, t := range targets {
		have := len(existing[[2]string{t.cuisine, t.course}])
		for i := have; i < t.want; i++ {
			jobs = append(jobs, job{t.cuisine, t.course})
		}
		fmt.Printf("%-4s %-9s have %2d / want %2d\n", t.cuisine, t.course, have, t.want)
	}
	fmt.Printf("total deficit: %d recipes\n", len(jobs))
	if dry || len(jobs) == 0 {
		return nil
	}
	if len(jobs) > maxN {
		fmt.Printf("capping this run at %d (re-run to continue)\n", maxN)
		jobs = jobs[:maxN]
	}

	client, err := ai.NewFromEnv()
	if err != nil {
		return err
	}

	generated, failed := 0, 0
	for i, j := range jobs {
		avoid := existing[[2]string{j.cuisine, j.course}]
		fmt.Printf("[%d/%d] generating %s %s … ", i+1, len(jobs), j.cuisine, j.course)
		f, err := client.GenerateRecipe(ctx, ai.GenSpec{
			Cuisine:          j.cuisine,
			Course:           j.course,
			AvoidNames:       avoid,
			KnownIngredients: knownList,
		}, known)
		if err != nil {
			failed++
			fmt.Println("FAILED:", err)
			continue
		}
		name := f.Recipes[0].Name
		if _, _, err := seeds.Import(ctx, f, "ai", "pending_review"); err != nil {
			failed++
			fmt.Println("IMPORT FAILED:", err)
			continue
		}
		// Track new names/ingredients so later jobs avoid duplicates.
		key := [2]string{j.cuisine, j.course}
		existing[key] = append(existing[key], name)
		for _, ing := range f.Ingredients {
			if !known[ing.CanonicalName] {
				known[ing.CanonicalName] = true
				knownList = append(knownList, ing.CanonicalName)
			}
			for _, a := range ing.Aliases {
				known[a] = true
			}
		}
		generated++
		fmt.Println("ok:", name)
	}
	fmt.Printf("\ndone: %d generated (pending_review), %d failed\n", generated, failed)
	if generated > 0 {
		fmt.Println("next: go run ./cmd/expand -review   # spot-check, then -activate")
	}
	return nil
}
