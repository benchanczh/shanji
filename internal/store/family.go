package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FamilyStore manages members, diet rules and the recipe catalog view.
type FamilyStore struct {
	pool *pgxpool.Pool
}

func NewFamilyStore(pool *pgxpool.Pool) *FamilyStore {
	return &FamilyStore{pool: pool}
}

// ---- members ----

type MemberView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Age  *int   `json:"age"`
	Role string `json:"role"`
}

func (s *FamilyStore) ListMembers(ctx context.Context, householdID int64) ([]MemberView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, age, role FROM members
		WHERE household_id = $1 ORDER BY id`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberView{}
	for rows.Next() {
		var m MemberView
		if err := rows.Scan(&m.ID, &m.Name, &m.Age, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *FamilyStore) CreateMember(ctx context.Context, householdID int64, name string, age *int, role string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO members (household_id, name, age, role)
		VALUES ($1, $2, $3, $4) RETURNING id`, householdID, name, age, role).Scan(&id)
	return id, err
}

func (s *FamilyStore) DeleteMember(ctx context.Context, householdID, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM members WHERE id = $1 AND household_id = $2`, id, householdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- diet rules ----

type DietRuleView struct {
	ID             int64   `json:"id"`
	MemberID       *int64  `json:"member_id"`
	Type           string  `json:"type"`
	Severity       string  `json:"severity"`
	IngredientID   *int64  `json:"ingredient_id"`
	IngredientName *string `json:"ingredient_name"`
	Tag            *string `json:"tag"`
	Note           *string `json:"note"`
}

func (s *FamilyStore) ListRules(ctx context.Context, householdID int64) ([]DietRuleView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dr.id, dr.member_id, dr.type, dr.severity, dr.ingredient_id, i.canonical_name, dr.tag, dr.note
		FROM diet_rules dr
		LEFT JOIN ingredients i ON i.id = dr.ingredient_id
		WHERE dr.household_id = $1 ORDER BY dr.id`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DietRuleView{}
	for rows.Next() {
		var v DietRuleView
		if err := rows.Scan(&v.ID, &v.MemberID, &v.Type, &v.Severity, &v.IngredientID, &v.IngredientName, &v.Tag, &v.Note); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResolveIngredient finds an ingredient by canonical name or alias —
// the same E3 discipline the seed importer enforces.
func (s *FamilyStore) ResolveIngredient(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM ingredients WHERE canonical_name = $1
		UNION
		SELECT ingredient_id FROM ingredient_aliases WHERE alias = $1
		LIMIT 1`, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("ingredient %q not in master data: %w", name, ErrNotFound)
	}
	return id, err
}

func (s *FamilyStore) CreateRule(ctx context.Context, householdID int64, memberID *int64, ruleType, severity string, ingredientID *int64, tag, note *string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO diet_rules (household_id, member_id, type, severity, ingredient_id, tag, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		householdID, memberID, ruleType, severity, ingredientID, tag, note).Scan(&id)
	return id, err
}

func (s *FamilyStore) DeleteRule(ctx context.Context, householdID, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM diet_rules WHERE id = $1 AND household_id = $2`, id, householdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- recipe catalog ----

type RecipeListItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	NameEN        string `json:"name_en"`
	Cuisine       string `json:"cuisine"`
	Course        string `json:"course"`
	Minutes       int    `json:"minutes"`
	Difficulty    string `json:"difficulty"`
	ProteinType   string `json:"protein_type"`
	BabyAdaptable bool   `json:"baby_adaptable"`
	Source        string `json:"source"`
}

func (s *FamilyStore) ListRecipes(ctx context.Context, course, cuisine, query string) ([]RecipeListItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, name_en, cuisine, course, minutes, difficulty, protein_type, baby_adaptable, source
		FROM recipes
		WHERE status = 'active'
		  AND ($1 = '' OR course = $1)
		  AND ($2 = '' OR cuisine = $2)
		  AND ($3 = '' OR name ILIKE '%' || $3 || '%' OR name_en ILIKE '%' || $3 || '%')
		ORDER BY cuisine, course, name`, course, cuisine, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecipeListItem{}
	for rows.Next() {
		var r RecipeListItem
		if err := rows.Scan(&r.ID, &r.Name, &r.NameEN, &r.Cuisine, &r.Course, &r.Minutes, &r.Difficulty, &r.ProteinType, &r.BabyAdaptable, &r.Source); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
