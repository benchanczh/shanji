// Package seedjson defines the recipe-library data contract shared by
// the manual seed importer and the AI generation pipeline. Every
// producer goes through the same Validate gate (E3 discipline: no
// free-text ingredients).
package seedjson

import (
	"encoding/json"
	"fmt"
	"strings"
)

type File struct {
	Ingredients []Ingredient `json:"ingredients"`
	Recipes     []Recipe     `json:"recipes"`
}

type Ingredient struct {
	CanonicalName string   `json:"canonical_name"`
	NameEN        string   `json:"name_en"`
	Category      string   `json:"category"`
	DefaultUnit   string   `json:"default_unit"`
	Aliases       []string `json:"aliases"`
}

type Recipe struct {
	Name          string         `json:"name"`
	NameEN        string         `json:"name_en"`
	Cuisine       string         `json:"cuisine"`
	Course        string         `json:"course"`
	Minutes       int            `json:"minutes"`
	Difficulty    string         `json:"difficulty"`
	ProteinType   string         `json:"protein_type"`
	NutritionTags []string       `json:"nutrition_tags"`
	BabyAdaptable bool           `json:"baby_adaptable"`
	Ingredients   []RecipeIngred `json:"ingredients"`
	Steps         []Step         `json:"steps"`
}

type RecipeIngred struct {
	Name string   `json:"name"`
	Qty  *float64 `json:"qty"` // nil = 适量, excluded from shopping aggregation
	Unit string   `json:"unit"`
	Note string   `json:"note,omitempty"`
}

type Step struct {
	CN             string `json:"cn"`
	EN             string `json:"en"`
	BabySplitPoint bool   `json:"baby_split_point,omitempty"`
}

var validCourses = map[string]bool{"main": true, "side": true, "soup": true, "breakfast": true}
var validProteins = map[string]bool{"pork": true, "chicken": true, "beef": true, "fish": true, "shrimp": true, "egg": true, "tofu": true, "none": true}
var validDifficulties = map[string]bool{"easy": true, "medium": true, "hard": true}

// Parse decodes strictly (unknown fields rejected).
func Parse(data []byte) (*File, error) {
	var f File
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse seed json: %w", err)
	}
	return &f, nil
}

// Validate enforces the contract. `known` is the existing ingredient
// vocabulary (canonical names and aliases already in the database);
// ingredients defined in the file extend it.
func Validate(f *File, known map[string]bool) error {
	vocab := map[string]bool{}
	for k := range known {
		vocab[k] = true
	}
	for _, ing := range f.Ingredients {
		if ing.CanonicalName == "" {
			return fmt.Errorf("ingredient with empty canonical_name")
		}
		vocab[ing.CanonicalName] = true
		for _, a := range ing.Aliases {
			vocab[a] = true
		}
	}
	for _, r := range f.Recipes {
		if r.Name == "" {
			return fmt.Errorf("recipe with empty name")
		}
		if !validCourses[r.Course] {
			return fmt.Errorf("recipe %q: invalid course %q", r.Name, r.Course)
		}
		if !validProteins[r.ProteinType] {
			return fmt.Errorf("recipe %q: invalid protein_type %q", r.Name, r.ProteinType)
		}
		if !validDifficulties[r.Difficulty] {
			return fmt.Errorf("recipe %q: invalid difficulty %q", r.Name, r.Difficulty)
		}
		if r.Minutes <= 0 || r.Minutes > 600 {
			return fmt.Errorf("recipe %q: implausible minutes %d", r.Name, r.Minutes)
		}
		if len(r.Steps) < 2 || len(r.Steps) > 12 {
			return fmt.Errorf("recipe %q: needs 2-12 steps, has %d", r.Name, len(r.Steps))
		}
		if len(r.Ingredients) == 0 {
			return fmt.Errorf("recipe %q: no ingredients", r.Name)
		}
		for _, ri := range r.Ingredients {
			if !vocab[ri.Name] {
				return fmt.Errorf("recipe %q: ingredient %q not in master data (E3 violation)", r.Name, ri.Name)
			}
			if ri.Qty != nil && (*ri.Qty <= 0 || *ri.Qty > 10000) {
				return fmt.Errorf("recipe %q: implausible qty %v for %q", r.Name, *ri.Qty, ri.Name)
			}
		}
		for i, st := range r.Steps {
			if strings.TrimSpace(st.CN) == "" {
				return fmt.Errorf("recipe %q: step %d has empty Chinese text", r.Name, i+1)
			}
		}
	}
	return nil
}
