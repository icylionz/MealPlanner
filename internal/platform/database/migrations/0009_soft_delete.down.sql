DROP INDEX IF EXISTS grocery_items_live_idx;
DROP INDEX IF EXISTS grocery_lists_live_idx;
DROP INDEX IF EXISTS meal_plan_live_idx;
DROP INDEX IF EXISTS foods_live_idx;

ALTER TABLE grocery_items DROP COLUMN deleted_at;
ALTER TABLE grocery_lists DROP COLUMN deleted_at;
ALTER TABLE meal_plan     DROP COLUMN deleted_at;
ALTER TABLE foods         DROP COLUMN deleted_at;
