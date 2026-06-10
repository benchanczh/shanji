CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- 家庭与成员
-- ============================================================
CREATE TABLE households (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    primary_cuisine   TEXT,
    secondary_cuisine TEXT,
    cuisine_ratio     INT NOT NULL DEFAULT 60 CHECK (cuisine_ratio BETWEEN 0 AND 100),
    -- 餐次构成模板：{"lunch":{"main":1,"side":1,"soup":"optional"},"dinner":{...},"breakfast":{"single":1}}
    meal_template     JSONB NOT NULL DEFAULT '{
        "breakfast": {"single": 1},
        "lunch":     {"main": 1, "side": 1, "soup": "optional"},
        "dinner":    {"main": 1, "side": 1, "soup": "optional"}
    }',
    serving_factor    NUMERIC(4,2) NOT NULL DEFAULT 3.30,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER households_updated_at BEFORE UPDATE ON households
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE accounts (
    id            BIGSERIAL PRIMARY KEY,
    household_id  BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'decider' CHECK (role IN ('decider', 'spouse')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE members (
    id           BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    age          INT,
    role         TEXT NOT NULL CHECK (role IN ('decider', 'spouse', 'child', 'helper')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 食材主数据（过敏过滤与清单合并的地基, E3）
-- ============================================================
CREATE TABLE ingredients (
    id             BIGSERIAL PRIMARY KEY,
    canonical_name TEXT NOT NULL UNIQUE,
    name_en        TEXT NOT NULL DEFAULT '',
    category       TEXT NOT NULL DEFAULT 'other',  -- meat/seafood/vegetable/staple/condiment/dairy/fruit/other
    default_unit   TEXT NOT NULL DEFAULT 'g',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ingredient_aliases (
    id            BIGSERIAL PRIMARY KEY,
    ingredient_id BIGINT NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    alias         TEXT NOT NULL UNIQUE
);
CREATE INDEX idx_ingredient_aliases_ingredient ON ingredient_aliases(ingredient_id);

CREATE TABLE diet_rules (
    id            BIGSERIAL PRIMARY KEY,
    household_id  BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    member_id     BIGINT REFERENCES members(id) ON DELETE CASCADE,  -- NULL = 全家
    type          TEXT NOT NULL CHECK (type IN ('allergy', 'forbidden', 'baby', 'health', 'taste')),
    severity      TEXT NOT NULL DEFAULT 'hard' CHECK (severity IN ('hard', 'soft')),
    ingredient_id BIGINT REFERENCES ingredients(id) ON DELETE CASCADE,  -- 过敏/忌口挂食材
    tag           TEXT,                                                 -- 口味类规则挂 recipe tag
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ingredient_id IS NOT NULL OR tag IS NOT NULL)
);
CREATE INDEX idx_diet_rules_household ON diet_rules(household_id);

-- ============================================================
-- 食谱
-- ============================================================
CREATE TABLE recipes (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL,
    name_en        TEXT NOT NULL DEFAULT '',
    cuisine        TEXT NOT NULL DEFAULT 'home',  -- 粤/川/湘/江浙/home(通用家常)...
    course         TEXT NOT NULL CHECK (course IN ('main', 'side', 'soup', 'breakfast')),
    source         TEXT NOT NULL DEFAULT 'library' CHECK (source IN ('library', 'ai', 'external')),
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending_review', 'archived')),
    minutes        INT NOT NULL DEFAULT 30,
    difficulty     TEXT NOT NULL DEFAULT 'easy' CHECK (difficulty IN ('easy', 'medium', 'hard')),
    protein_type   TEXT NOT NULL DEFAULT 'none' CHECK (protein_type IN ('pork', 'chicken', 'beef', 'fish', 'shrimp', 'egg', 'tofu', 'none')),
    nutrition_tags JSONB NOT NULL DEFAULT '[]',
    baby_adaptable BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER recipes_updated_at BEFORE UPDATE ON recipes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE INDEX idx_recipes_course_status ON recipes(course, status);
CREATE INDEX idx_recipes_cuisine ON recipes(cuisine);

CREATE TABLE recipe_ingredients (
    id            BIGSERIAL PRIMARY KEY,
    recipe_id     BIGINT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    ingredient_id BIGINT NOT NULL REFERENCES ingredients(id),
    qty           NUMERIC(10,2),  -- NULL = 适量，不参与清单合并
    unit          TEXT NOT NULL DEFAULT 'g',
    note          TEXT
);
CREATE INDEX idx_recipe_ingredients_recipe ON recipe_ingredients(recipe_id);
CREATE INDEX idx_recipe_ingredients_ingredient ON recipe_ingredients(ingredient_id);

CREATE TABLE recipe_steps (
    id               BIGSERIAL PRIMARY KEY,
    recipe_id        BIGINT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    step_order       INT NOT NULL,
    text_cn          TEXT NOT NULL,
    text_en          TEXT NOT NULL DEFAULT '',
    image_url        TEXT,
    baby_split_point BOOLEAN NOT NULL DEFAULT false,  -- 此步之前分出宝宝份(未调味)
    UNIQUE (recipe_id, step_order)
);

-- ============================================================
-- 周计划
-- ============================================================
CREATE TABLE weekly_plans (
    id           BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    week_start   DATE NOT NULL,
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'confirmed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (household_id, week_start)
);
CREATE TRIGGER weekly_plans_updated_at BEFORE UPDATE ON weekly_plans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE meal_slots (
    id        BIGSERIAL PRIMARY KEY,
    plan_id   BIGINT NOT NULL REFERENCES weekly_plans(id) ON DELETE CASCADE,
    day       DATE NOT NULL,
    meal_type TEXT NOT NULL CHECK (meal_type IN ('breakfast', 'lunch', 'dinner')),
    locked    BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (plan_id, day, meal_type)
);

CREATE TABLE meal_dishes (
    id               BIGSERIAL PRIMARY KEY,
    slot_id          BIGINT NOT NULL REFERENCES meal_slots(id) ON DELETE CASCADE,
    recipe_id        BIGINT NOT NULL REFERENCES recipes(id),
    target           TEXT NOT NULL DEFAULT 'adult' CHECK (target IN ('adult', 'baby')),
    course           TEXT NOT NULL CHECK (course IN ('main', 'side', 'soup', 'breakfast')),
    reuse_of_dish_id BIGINT REFERENCES meal_dishes(id)  -- 复用餐(一锅两顿): 指向源菜, 清单聚合跳过
);
CREATE INDEX idx_meal_dishes_slot ON meal_dishes(slot_id);

-- ============================================================
-- 买菜清单
-- ============================================================
CREATE TABLE shopping_lists (
    id      BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES weekly_plans(id) ON DELETE CASCADE,
    version INT NOT NULL DEFAULT 1,
    UNIQUE (plan_id, version)
);

CREATE TABLE shopping_items (
    id            BIGSERIAL PRIMARY KEY,
    list_id       BIGINT NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    ingredient_id BIGINT NOT NULL REFERENCES ingredients(id),
    name          TEXT NOT NULL,
    name_en       TEXT NOT NULL DEFAULT '',
    total_qty     NUMERIC(10,2),  -- NULL = 适量
    unit          TEXT NOT NULL DEFAULT 'g',
    category      TEXT NOT NULL DEFAULT 'other',
    checked       BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_shopping_items_list ON shopping_items(list_id);

-- ============================================================
-- 协作与访问
-- ============================================================
CREATE TABLE suggestions (
    id           BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    from_role    TEXT NOT NULL DEFAULT 'helper' CHECK (from_role IN ('helper', 'spouse')),
    title        TEXT NOT NULL,
    content      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE maid_links (
    id           BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    scopes       TEXT[] NOT NULL DEFAULT '{today:read,suggestions:write}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);

-- ============================================================
-- 种子数据：单家庭 + 管理账号 (admin / admin123)
-- ============================================================
INSERT INTO households (name) VALUES ('我的家');
INSERT INTO accounts (household_id, username, password_hash, role)
VALUES (1, 'admin', crypt('admin123', gen_salt('bf')), 'decider');
