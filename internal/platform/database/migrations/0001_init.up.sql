-- MealPlanner initial schema. Mirrors docs/doc.md section 6.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 6.1 Identity and households ------------------------------------------------

CREATE TABLE accounts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,
    name          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX accounts_email_lower_key ON accounts (lower(email));

CREATE TABLE households (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    is_template boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE household_members (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    account_id   uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    role         text NOT NULL CHECK (role IN ('owner', 'member')),
    initials     text NOT NULL DEFAULT '',
    color        text NOT NULL DEFAULT '#22386A',
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, account_id)
);
CREATE INDEX household_members_account_idx ON household_members (account_id);

CREATE TABLE sessions (
    token               text PRIMARY KEY,
    account_id          uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    active_household_id uuid REFERENCES households (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL
);
CREATE INDEX sessions_account_idx ON sessions (account_id);

CREATE TABLE invites (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    code         text NOT NULL UNIQUE,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    max_uses     integer CHECK (max_uses IS NULL OR max_uses > 0),
    use_count    integer NOT NULL DEFAULT 0,
    created_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX invites_household_idx ON invites (household_id);

-- 6.2 Unified foods ------------------------------------------------------------

CREATE TABLE foods (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id            uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name                    text NOT NULL,
    description             text NOT NULL DEFAULT '',
    prep_time_min           integer NOT NULL DEFAULT 0 CHECK (prep_time_min >= 0),
    cook_time_min           integer NOT NULL DEFAULT 0 CHECK (cook_time_min >= 0),
    servings                double precision NOT NULL DEFAULT 1 CHECK (servings > 0),
    yield_amount            double precision CHECK (yield_amount IS NULL OR yield_amount > 0),
    default_unit            text NOT NULL DEFAULT '',
    density_g_per_ml        double precision CHECK (density_g_per_ml IS NULL OR density_g_per_ml > 0),
    density_source          text NOT NULL DEFAULT 'none' CHECK (density_source IN ('starter', 'custom', 'none')),
    source_url              text NOT NULL DEFAULT '',
    source_last_imported_at timestamptz,
    version                 integer NOT NULL DEFAULT 1,
    created_by              uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by              uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz
);
CREATE INDEX foods_household_idx ON foods (household_id) WHERE deleted_at IS NULL;
CREATE INDEX foods_name_lower_idx ON foods (household_id, lower(name));

CREATE TABLE food_aliases (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    food_id    uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    alias      text NOT NULL,
    created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX food_aliases_food_idx ON food_aliases (food_id);
CREATE INDEX food_aliases_lower_idx ON food_aliases (lower(alias));

CREATE TABLE food_tags (
    food_id uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    tag     text NOT NULL,
    PRIMARY KEY (food_id, tag)
);
CREATE INDEX food_tags_tag_idx ON food_tags (tag);

CREATE TABLE food_components (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_food_id uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    child_food_id  uuid NOT NULL REFERENCES foods (id),
    amount         double precision NOT NULL CHECK (amount > 0),
    unit           text NOT NULL DEFAULT '',
    variant_text   text NOT NULL DEFAULT '',
    sort_order     integer NOT NULL DEFAULT 0,
    CHECK (parent_food_id <> child_food_id)
);
CREATE INDEX food_components_parent_idx ON food_components (parent_food_id, sort_order);
CREATE INDEX food_components_child_idx ON food_components (child_food_id);

CREATE TABLE food_steps (
    food_id     uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    step_number integer NOT NULL,
    instruction text NOT NULL,
    PRIMARY KEY (food_id, step_number)
);

-- 6.3 Planning ---------------------------------------------------------------

CREATE TABLE meal_series (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    food_id      uuid NOT NULL REFERENCES foods (id),
    plan_time    text NOT NULL CHECK (plan_time ~ '^[0-2][0-9]:[0-5][0-9]$'),
    servings     double precision NOT NULL CHECK (servings > 0),
    freq         text NOT NULL CHECK (freq IN ('daily', 'weekly')),
    byweekday    text NOT NULL DEFAULT '',
    start_date   date NOT NULL,
    until_date   date NOT NULL,
    version      integer NOT NULL DEFAULT 1,
    CHECK (until_date >= start_date)
);

CREATE TABLE meal_plan (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    plan_date    date NOT NULL,
    plan_time    text NOT NULL CHECK (plan_time ~ '^[0-2][0-9]:[0-5][0-9]$'),
    food_id      uuid NOT NULL REFERENCES foods (id),
    servings     double precision NOT NULL CHECK (servings > 0),
    series_id    uuid REFERENCES meal_series (id) ON DELETE SET NULL,
    title        text NOT NULL DEFAULT '',
    notes        text NOT NULL DEFAULT '',
    link_url     text NOT NULL DEFAULT '',
    version      integer NOT NULL DEFAULT 1,
    created_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    deleted_at   timestamptz
);
CREATE INDEX meal_plan_household_date_idx ON meal_plan (household_id, plan_date, plan_time) WHERE deleted_at IS NULL;
CREATE INDEX meal_plan_series_idx ON meal_plan (series_id);

CREATE TABLE scheduled_meal_recipes (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    meal_id           uuid NOT NULL REFERENCES meal_plan (id) ON DELETE CASCADE,
    food_id           uuid NOT NULL REFERENCES foods (id),
    servings_override double precision CHECK (servings_override IS NULL OR servings_override > 0),
    sort_order        integer NOT NULL DEFAULT 0,
    created_by        uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by        uuid REFERENCES accounts (id) ON DELETE SET NULL
);
CREATE INDEX scheduled_meal_recipes_meal_idx ON scheduled_meal_recipes (meal_id, sort_order);

-- 6.4 Grocery lists ----------------------------------------------------------

CREATE TABLE grocery_lists (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name         text NOT NULL,
    created_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);
CREATE INDEX grocery_lists_household_idx ON grocery_lists (household_id) WHERE deleted_at IS NULL;

CREATE TABLE grocery_items (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    list_id          uuid NOT NULL REFERENCES grocery_lists (id) ON DELETE CASCADE,
    ingredient_id    uuid REFERENCES foods (id),
    name             text NOT NULL,
    amount           double precision NOT NULL DEFAULT 0 CHECK (amount >= 0),
    unit             text NOT NULL DEFAULT '',
    variant_text     text NOT NULL DEFAULT '',
    note             text NOT NULL DEFAULT '',
    sort_order       integer NOT NULL DEFAULT 0,
    cost             double precision CHECK (cost IS NULL OR cost >= 0),
    cost_recorded_at timestamptz,
    checked          boolean NOT NULL DEFAULT false,
    source_type      text NOT NULL DEFAULT 'adhoc' CHECK (source_type IN ('generated', 'adhoc')),
    created_by       uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by       uuid REFERENCES accounts (id) ON DELETE SET NULL,
    deleted_at       timestamptz
);
CREATE INDEX grocery_items_list_idx ON grocery_items (list_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX grocery_items_cost_idx ON grocery_items (ingredient_id, cost_recorded_at DESC) WHERE cost IS NOT NULL;

CREATE TABLE grocery_item_sources (
    id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    grocery_item_id           uuid NOT NULL REFERENCES grocery_items (id) ON DELETE CASCADE,
    scheduled_meal_id         uuid REFERENCES meal_plan (id) ON DELETE SET NULL,
    recipe_id                 uuid REFERENCES foods (id),
    recipe_ingredient_line_id uuid REFERENCES food_components (id) ON DELETE SET NULL,
    quantity_contributed      double precision NOT NULL,
    unit_contributed          text NOT NULL DEFAULT '',
    variant_text              text NOT NULL DEFAULT '',
    line_recipe_name          text NOT NULL DEFAULT ''
);
CREATE INDEX grocery_item_sources_item_idx ON grocery_item_sources (grocery_item_id);

-- 6.5 Prep -------------------------------------------------------------------

CREATE TABLE prep_sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name         text NOT NULL,
    session_date date NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX prep_sessions_household_idx ON prep_sessions (household_id, session_date);

CREATE TABLE prep_session_meals (
    session_id uuid NOT NULL REFERENCES prep_sessions (id) ON DELETE CASCADE,
    food_id    uuid NOT NULL REFERENCES foods (id),
    servings   double precision NOT NULL CHECK (servings > 0),
    sort_order integer NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, food_id)
);

-- Starter Template -----------------------------------------------------------
-- A hidden household whose foods are cloned into every new household. The
-- optional cmd/seed-foods command enriches it from CoFID.

INSERT INTO households (id, name, is_template)
VALUES ('00000000-0000-0000-0000-000000000001', 'Starter Template', true);

INSERT INTO foods (household_id, name, default_unit, density_g_per_ml, density_source)
SELECT '00000000-0000-0000-0000-000000000001', v.name, v.unit, v.density,
       CASE WHEN v.density IS NULL THEN 'none' ELSE 'starter' END
FROM (VALUES
    ('All-purpose flour', 'g', 0.53),
    ('Bread flour', 'g', 0.55),
    ('Granulated sugar', 'g', 0.85),
    ('Brown sugar', 'g', 0.93),
    ('Icing sugar', 'g', 0.56),
    ('Salt', 'g', 1.2),
    ('Black pepper', 'g', 0.5),
    ('Baking powder', 'g', 0.9),
    ('Baking soda', 'g', 0.95),
    ('Butter', 'g', 0.96),
    ('Olive oil', 'ml', 0.91),
    ('Vegetable oil', 'ml', 0.92),
    ('Whole milk', 'ml', 1.03),
    ('Heavy cream', 'ml', 1.01),
    ('Plain yogurt', 'g', 1.03),
    ('Water', 'ml', 1.0),
    ('Honey', 'g', 1.42),
    ('Maple syrup', 'ml', 1.32),
    ('Soy sauce', 'ml', 1.15),
    ('White rice', 'g', 0.85),
    ('Rolled oats', 'g', 0.41),
    ('Cocoa powder', 'g', 0.42),
    ('Grated parmesan', 'g', 0.42),
    ('Cheddar cheese', 'g', NULL),
    ('Eggs', 'each', NULL),
    ('Onion', 'each', NULL),
    ('Garlic', 'clove', NULL),
    ('Carrot', 'each', NULL),
    ('Tomato', 'each', NULL),
    ('Potato', 'g', NULL),
    ('Lemon', 'each', NULL),
    ('Chicken breast', 'g', NULL),
    ('Ground beef', 'g', NULL),
    ('Spaghetti', 'g', NULL),
    ('Canned tomatoes', 'g', 1.03),
    ('Chicken stock', 'ml', 1.0),
    ('Fresh basil', 'g', NULL),
    ('Ground cumin', 'g', 0.45),
    ('Paprika', 'g', 0.46),
    ('Dried oregano', 'g', 0.25)
) AS v (name, unit, density);
