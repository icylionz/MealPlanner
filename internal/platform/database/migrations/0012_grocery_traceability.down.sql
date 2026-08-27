DROP TABLE IF EXISTS grocery_item_sources;

DROP INDEX IF EXISTS grocery_items_ingredient_idx;

ALTER TABLE grocery_items
    DROP COLUMN IF EXISTS source_type,
    DROP COLUMN IF EXISTS ingredient_id;
