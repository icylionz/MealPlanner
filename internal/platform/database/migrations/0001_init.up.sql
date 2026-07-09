CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE household_members (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    role       text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    initials   text NOT NULL,
    color      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token      text PRIMARY KEY,
    member_id  uuid NOT NULL REFERENCES household_members (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE TABLE recipes (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    prep_time_min int  NOT NULL DEFAULT 0,
    cook_time_min int  NOT NULL DEFAULT 0,
    servings      int  NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recipe_tags (
    recipe_id uuid NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    tag       text NOT NULL,
    PRIMARY KEY (recipe_id, tag)
);

-- An ingredient line is either a raw ingredient (name set, sub_recipe_id null)
-- or a sub-recipe reference (sub_recipe_id set).
CREATE TABLE recipe_ingredients (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_id     uuid NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    name          text NOT NULL DEFAULT '',
    amount        numeric NOT NULL DEFAULT 0,
    unit          text NOT NULL DEFAULT 'g',
    sub_recipe_id uuid REFERENCES recipes (id) ON DELETE RESTRICT,
    sort_order    int NOT NULL DEFAULT 0,
    CHECK (sub_recipe_id IS NOT NULL OR name <> '')
);

CREATE INDEX recipe_ingredients_recipe_idx ON recipe_ingredients (recipe_id, sort_order);

CREATE TABLE recipe_steps (
    recipe_id   uuid NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    step_number int  NOT NULL,
    instruction text NOT NULL,
    PRIMARY KEY (recipe_id, step_number)
);

-- Times are stored as 'HH:MM' text: the prototype sorts and compares them
-- lexicographically, which is exact for zero-padded 24h times.
CREATE TABLE meal_plan (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_date date NOT NULL,
    plan_time text NOT NULL DEFAULT '12:00' CHECK (plan_time ~ '^[0-2][0-9]:[0-5][0-9]$'),
    recipe_id uuid NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    servings  int  NOT NULL DEFAULT 2
);

CREATE INDEX meal_plan_date_idx ON meal_plan (plan_date, plan_time);

CREATE TABLE grocery_lists (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE grocery_items (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    list_id    uuid NOT NULL REFERENCES grocery_lists (id) ON DELETE CASCADE,
    name       text NOT NULL,
    amount     numeric NOT NULL DEFAULT 0,
    unit       text NOT NULL DEFAULT 'g',
    checked    boolean NOT NULL DEFAULT false,
    note       text NOT NULL DEFAULT '',
    sort_order int NOT NULL DEFAULT 0
);

CREATE INDEX grocery_items_list_idx ON grocery_items (list_id, sort_order);

CREATE TABLE prep_sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name         text NOT NULL,
    session_date date NOT NULL DEFAULT current_date,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE prep_session_meals (
    session_id uuid NOT NULL REFERENCES prep_sessions (id) ON DELETE CASCADE,
    recipe_id  uuid NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    servings   int  NOT NULL DEFAULT 2,
    sort_order int  NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, recipe_id)
);
