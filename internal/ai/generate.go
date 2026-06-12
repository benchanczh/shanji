package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/benchanczh/shanji/internal/seedjson"
)

// GenSpec describes one recipe to generate for library expansion.
type GenSpec struct {
	Cuisine    string
	Course     string
	AvoidNames []string // existing dishes in this cuisine+course — do not duplicate
	// KnownIngredients is the current master-data vocabulary; the
	// model should prefer it and explicitly define anything new.
	KnownIngredients []string
}

const genSystem = `你是一位经验丰富的中餐家庭料理顾问，为一个家庭膳食规划应用生成结构化食谱。
要求：
- 必须是真实、常见、做法可靠的家常菜，步骤要符合实际烹饪顺序，火候和时间要合理。
- 步骤 2-6 步，每步一句话说清楚；中英双语，英文面向不熟悉中餐的菲律宾帮佣，用简单清晰的英语。
- 食材优先使用提供的词表名称；词表里没有的食材必须在 ingredients 数组中完整定义（canonical_name/name_en/category/default_unit/aliases）。
- category 只能是: meat/seafood/vegetable/fruit/dairy/staple/condiment/other。
- 调味料用量不确定时 qty 用 null 并把 unit 写为"适量"。
- 如果这道菜适合在调味前为 2 岁宝宝分出一份（少盐少油软烂），baby_adaptable 设为 true，并在对应步骤标 baby_split_point=true、步骤文字中说明分餐。
- 只输出 JSON，不要任何解释文字。`

const genPromptTemplate = `请生成一道%s的%s（course=%s）。

不要与这些已有菜品重复（也不要只是它们的微小变体）：%s

现有食材词表（优先使用这些名称）：
%s

输出这个 JSON 结构（ingredients 只放词表里没有的新食材，没有就给空数组）：
{
  "ingredients": [
    {"canonical_name": "", "name_en": "", "category": "", "default_unit": "", "aliases": []}
  ],
  "recipes": [
    {
      "name": "", "name_en": "", "cuisine": "%s", "course": "%s",
      "minutes": 30, "difficulty": "easy", "protein_type": "pork|chicken|beef|fish|shrimp|egg|tofu|none",
      "nutrition_tags": [], "baby_adaptable": false,
      "ingredients": [{"name": "", "qty": 100, "unit": "g", "note": ""}],
      "steps": [{"cn": "", "en": "", "baby_split_point": false}]
    }
  ]
}`

var courseCN = map[string]string{"main": "荤主菜", "side": "素菜", "soup": "汤", "breakfast": "早餐"}

// GenerateRecipe asks the model for one recipe and validates it
// against the seedjson contract. Invalid output retries up to 2 times
// with the validation error fed back.
func (c *Client) GenerateRecipe(ctx context.Context, spec GenSpec, known map[string]bool) (*seedjson.File, error) {
	prompt := fmt.Sprintf(genPromptTemplate,
		spec.Cuisine, courseCN[spec.Course], spec.Course,
		strings.Join(spec.AvoidNames, "、"),
		strings.Join(spec.KnownIngredients, "、"),
		spec.Cuisine, spec.Course,
	)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		p := prompt
		if lastErr != nil {
			p += fmt.Sprintf("\n\n上一次输出未通过校验：%v。请修正后重新输出完整 JSON。", lastErr)
		}
		text, err := c.complete(ctx, c.modelGen, genSystem, p, 4096)
		if err != nil {
			return nil, fmt.Errorf("llm call: %w", err) // transport errors don't retry here
		}
		raw := ExtractJSON(text)
		if raw == "" {
			lastErr = fmt.Errorf("no JSON object in response")
			continue
		}
		f, err := seedjson.Parse([]byte(raw))
		if err != nil {
			lastErr = err
			continue
		}
		if len(f.Recipes) != 1 {
			lastErr = fmt.Errorf("expected exactly 1 recipe, got %d", len(f.Recipes))
			continue
		}
		// Pin the requested classification regardless of model drift.
		f.Recipes[0].Cuisine = spec.Cuisine
		f.Recipes[0].Course = spec.Course
		if err := seedjson.Validate(f, known); err != nil {
			lastErr = err
			continue
		}
		return f, nil
	}
	return nil, fmt.Errorf("generation failed after retries: %w", lastErr)
}
