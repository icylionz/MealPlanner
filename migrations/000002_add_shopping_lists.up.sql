-- Shopping Lists (basic container)
CREATE TABLE shopping_lists (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Shopping List Items (the actual items to buy)
CREATE TABLE shopping_list_items (
    id SERIAL PRIMARY KEY,
    shopping_list_id INTEGER REFERENCES shopping_lists(id) ON DELETE CASCADE,
    food_id INTEGER REFERENCES foods(id),
    food_name TEXT NOT NULL,
    unit TEXT NOT NULL,
    unit_type TEXT NOT NULL,
    notes TEXT,
    purchased BOOLEAN DEFAULT FALSE,
    actual_quantity NUMERIC,
    actual_price NUMERIC,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Source tracking (what contributed this item)
CREATE TABLE shopping_list_sources (
    id SERIAL PRIMARY KEY,
    shopping_list_id INTEGER REFERENCES shopping_lists(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL, -- 'schedule', 'recipe', 'manual', 'copy'
    source_id INTEGER, -- schedule_id, recipe_id, or null for manual
    source_name TEXT NOT NULL, -- Human readable source name
    servings NUMERIC, -- For recipes/schedules
    added_at TIMESTAMPTZ DEFAULT NOW()
);

-- Link items to their sources (many-to-many since consolidation can merge sources)
CREATE TABLE shopping_list_item_sources (
    shopping_list_item_id INTEGER REFERENCES shopping_list_items(id) ON DELETE CASCADE,
    shopping_list_source_id INTEGER REFERENCES shopping_list_sources(id) ON DELETE CASCADE,
    contributed_quantity NUMERIC NOT NULL,
    PRIMARY KEY (shopping_list_item_id, shopping_list_source_id)
);

-- Indexes
CREATE INDEX idx_shopping_list_items_list_id ON shopping_list_items(shopping_list_id);
CREATE INDEX idx_shopping_list_sources_list_id ON shopping_list_sources(shopping_list_id);
CREATE INDEX idx_shopping_list_item_sources_item ON shopping_list_item_sources(shopping_list_item_id);
