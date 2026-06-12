// Command seed imports curated recipe library content from a JSON
// file. The AI expansion pipeline (cmd/expand) produces the same
// contract and goes through the same validation and import path.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benchanczh/shanji/internal/config"
	"github.com/benchanczh/shanji/internal/seedjson"
	"github.com/benchanczh/shanji/internal/store"
)

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
	f, err := seedjson.Parse(raw)
	if err != nil {
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

	seeds := store.NewSeedStore(pool)
	known, err := seeds.KnownIngredientNames(ctx)
	if err != nil {
		return err
	}
	if err := seedjson.Validate(f, known); err != nil {
		return err
	}

	inserted, skipped, err := seeds.Import(ctx, f, "library", "active")
	if err != nil {
		return err
	}
	fmt.Printf("ingredients upserted: %d, recipes inserted: %d, skipped (already exist): %d\n",
		len(f.Ingredients), inserted, skipped)
	return nil
}
