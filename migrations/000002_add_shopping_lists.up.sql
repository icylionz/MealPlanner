-- Shopping Lists
CREATE TABLE shopping_lists (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT valid_date_range CHECK (end_date >= start_date)
);

-- Shopping List Items
CREATE TABLE shopping_list_items (
    id SERIAL PRIMARY KEY,
    shopping_list_id INTEGER REFERENCES shopping_lists(id) ON DELETE CASCADE,
    food_id INTEGER REFERENCES foods(id),
    quantity NUMERIC NOT NULL CHECK (quantity > 0),
    unit TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Shopping List Meals (tracks which meals are included in the list)
CREATE TABLE shopping_list_meals (
    id SERIAL PRIMARY KEY,
    shopping_list_id INTEGER REFERENCES shopping_lists(id) ON DELETE CASCADE,
    schedule_id INTEGER REFERENCES schedules(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(shopping_list_id, schedule_id)
);

-- Shopping List Purchases (for tracking actual purchases)
CREATE TABLE shopping_list_purchases (
    id SERIAL PRIMARY KEY,
    shopping_list_item_id INTEGER REFERENCES shopping_list_items(id) ON DELETE CASCADE,
    actual_quantity NUMERIC NOT NULL CHECK (actual_quantity > 0),
    price NUMERIC CHECK (price >= 0),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_shopping_list_items_list_id ON shopping_list_items(shopping_list_id);
CREATE INDEX idx_shopping_list_meals_list_id ON shopping_list_meals(shopping_list_id);
CREATE INDEX idx_shopping_list_purchases_item_id ON shopping_list_purchases(shopping_list_item_id);