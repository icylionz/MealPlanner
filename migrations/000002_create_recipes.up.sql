CREATE TABLE recipes (
    food_id INTEGER PRIMARY KEY REFERENCES foods (id) ON DELETE CASCADE,
    instructions TEXT,
    url TEXT,
    yield_quantity NUMERIC NOT NULL,
    yield_unit TEXT NOT NULL,
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
