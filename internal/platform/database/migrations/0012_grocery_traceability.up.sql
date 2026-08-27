-- G5: generated grocery items retain their canonical leaf food and every raw
-- contribution that formed the displayed total. Existing rows predate
-- provenance and are therefore ad-hoc.
ALTER TABLE grocery_items
    ADD COLUMN ingredient_id uuid REFERENCES foods (id) ON DELETE SET NULL,
    ADD COLUMN source_type text NOT NULL DEFAULT 'adhoc'
        CHECK (source_type IN ('generated', 'adhoc'));

CREATE INDEX grocery_items_ingredient_idx
    ON grocery_items (ingredient_id) WHERE ingredient_id IS NOT NULL;

CREATE TABLE grocery_item_sources (
    id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    grocery_item_id           uuid NOT NULL REFERENCES grocery_items (id) ON DELETE CASCADE,
    scheduled_meal_id         uuid REFERENCES meal_plan (id) ON DELETE SET NULL,
    recipe_id                 uuid REFERENCES foods (id) ON DELETE SET NULL,
    recipe_ingredient_line_id uuid REFERENCES food_components (id) ON DELETE SET NULL,
    quantity_contributed      numeric NOT NULL,
    unit_contributed          text NOT NULL
);

CREATE INDEX grocery_item_sources_item_idx
    ON grocery_item_sources (grocery_item_id, id);
