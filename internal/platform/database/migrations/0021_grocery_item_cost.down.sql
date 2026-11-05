ALTER TABLE grocery_items
    DROP CONSTRAINT grocery_items_cost_nonnegative,
    DROP COLUMN cost_recorded_at,
    DROP COLUMN cost;
