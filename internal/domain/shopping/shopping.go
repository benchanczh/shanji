// Package shopping aggregates a week plan's ingredients into a
// bilingual shopping list. Pure domain: no SQL, no HTTP.
package shopping

import (
	"sort"
)

// Line is one recipe-ingredient occurrence in the plan (one row per
// dish, so a recipe planned twice contributes two lines).
type Line struct {
	IngredientID int64
	Name         string
	NameEN       string
	Category     string
	Unit         string
	Qty          *float64 // nil = 适量 (to taste), excluded from sums
}

// Item is one aggregated shopping list entry.
type Item struct {
	IngredientID int64
	Name         string
	NameEN       string
	Category     string
	Unit         string
	TotalQty     *float64 // nil = 适量
}

// categoryOrder drives the market-walk grouping of the list.
var categoryOrder = map[string]int{
	"meat": 0, "seafood": 1, "vegetable": 2, "fruit": 3,
	"dairy": 4, "staple": 5, "condiment": 6, "other": 7,
}

// Aggregate merges lines into the final list:
//   - quantified lines sum per (ingredient, unit);
//   - 适量 lines collapse to one entry per ingredient, and are dropped
//     entirely when a quantified entry for the same ingredient exists
//     (the summed amount already covers the purchase);
//   - output is sorted by category (market walk order), then name.
func Aggregate(lines []Line) []Item {
	type key struct {
		ingredient int64
		unit       string
	}
	sums := map[key]*Item{}
	toTaste := map[int64]*Item{}
	hasQty := map[int64]bool{}

	for _, l := range lines {
		if l.Qty == nil {
			if _, ok := toTaste[l.IngredientID]; !ok {
				toTaste[l.IngredientID] = &Item{
					IngredientID: l.IngredientID, Name: l.Name, NameEN: l.NameEN,
					Category: l.Category, Unit: "适量",
				}
			}
			continue
		}
		hasQty[l.IngredientID] = true
		k := key{l.IngredientID, l.Unit}
		if it, ok := sums[k]; ok {
			*it.TotalQty += *l.Qty
		} else {
			qty := *l.Qty
			sums[k] = &Item{
				IngredientID: l.IngredientID, Name: l.Name, NameEN: l.NameEN,
				Category: l.Category, Unit: l.Unit, TotalQty: &qty,
			}
		}
	}

	var items []Item
	for _, it := range sums {
		items = append(items, *it)
	}
	for id, it := range toTaste {
		if !hasQty[id] {
			items = append(items, *it)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		ci, cj := categoryRank(items[i].Category), categoryRank(items[j].Category)
		if ci != cj {
			return ci < cj
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Unit < items[j].Unit
	})
	return items
}

func categoryRank(c string) int {
	if r, ok := categoryOrder[c]; ok {
		return r
	}
	return len(categoryOrder)
}
