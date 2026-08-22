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

-- A food is either atomic (no components) or a recipe (one or more components).
-- Recipe metadata (prep/cook/servings/steps) is only meaningful for foods that
-- have components, but every food may carry it.
CREATE TABLE foods (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    prep_time_min int  NOT NULL DEFAULT 0,
    cook_time_min int  NOT NULL DEFAULT 0,
    servings      int  NOT NULL DEFAULT 1,
    default_unit  text NOT NULL DEFAULT 'g',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE food_tags (
    food_id uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    tag     text NOT NULL,
    PRIMARY KEY (food_id, tag)
);

-- A component line always references another food (never free text). Atomic
-- foods used as components cannot be deleted while referenced (RESTRICT).
CREATE TABLE food_components (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_food_id uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    child_food_id  uuid NOT NULL REFERENCES foods (id) ON DELETE RESTRICT,
    amount        numeric NOT NULL DEFAULT 0,
    unit          text NOT NULL DEFAULT 'g',
    sort_order    int NOT NULL DEFAULT 0,
    CHECK (parent_food_id <> child_food_id)
);

CREATE INDEX food_components_parent_idx ON food_components (parent_food_id, sort_order);

CREATE TABLE food_steps (
    food_id     uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    step_number int  NOT NULL,
    instruction text NOT NULL,
    PRIMARY KEY (food_id, step_number)
);

-- Times are stored as 'HH:MM' text: the prototype sorts and compares them
-- lexicographically, which is exact for zero-padded 24h times.
CREATE TABLE meal_plan (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_date date NOT NULL,
    plan_time text NOT NULL DEFAULT '12:00' CHECK (plan_time ~ '^[0-2][0-9]:[0-5][0-9]$'),
    food_id   uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
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
    food_id    uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    servings   int  NOT NULL DEFAULT 2,
    sort_order int  NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, food_id)
);
