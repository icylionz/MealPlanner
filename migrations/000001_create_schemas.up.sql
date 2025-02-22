CREATE TABLE foods (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    unit_type TEXT NOT NULL,
    base_unit TEXT NOT NULL,
    density NUMERIC NULL,
    is_recipe BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW (),
    updated_at TIMESTAMPTZ DEFAULT NOW ()
);

CREATE TABLE recipes (
    food_id INTEGER PRIMARY KEY REFERENCES foods (id) ON DELETE CASCADE,
    instructions TEXT,
    url TEXT,
    yield_quantity NUMERIC NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW (),
    updated_at TIMESTAMPTZ DEFAULT NOW ()
);

CREATE TABLE recipe_ingredients (
    recipe_id INTEGER REFERENCES recipes (food_id) ON DELETE CASCADE,
    ingredient_id INTEGER REFERENCES foods (id),
    quantity NUMERIC NOT NULL,
    unit TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW (),
    PRIMARY KEY (recipe_id, ingredient_id)
);

CREATE TABLE schedules (
    id SERIAL PRIMARY KEY,
    food_id INTEGER REFERENCES foods (id) ON DELETE CASCADE,
    scheduled_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW (),
    updated_at TIMESTAMPTZ DEFAULT NOW ()
);

-- Add index for range queries
CREATE INDEX idx_schedules_scheduled_at ON schedules (scheduled_at);
