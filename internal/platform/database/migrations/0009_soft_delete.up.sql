-- G1: soft delete foundation. Deletes flip a deleted_at timestamp instead of
-- removing rows, so history stays readable: a past meal whose food was removed,
-- or a grocery item referencing a since-deleted food, still renders its name.
--
-- FK cascades are intentionally left unchanged. Soft delete issues UPDATE, never
-- DELETE, so a soft-deleted food's row persists and old meals/lists still join
-- to it. The existing ON DELETE CASCADE edges only fire on genuine row removal
-- (household teardown), which is the correct behavior to keep.
ALTER TABLE foods         ADD COLUMN deleted_at timestamptz;
ALTER TABLE meal_plan     ADD COLUMN deleted_at timestamptz;
ALTER TABLE grocery_lists ADD COLUMN deleted_at timestamptz;
ALTER TABLE grocery_items ADD COLUMN deleted_at timestamptz;

-- Partial indexes keep the common "live rows only" scans fast.
CREATE INDEX foods_live_idx         ON foods (household_id)             WHERE deleted_at IS NULL;
CREATE INDEX meal_plan_live_idx     ON meal_plan (household_id, plan_date) WHERE deleted_at IS NULL;
CREATE INDEX grocery_lists_live_idx ON grocery_lists (household_id)     WHERE deleted_at IS NULL;
CREATE INDEX grocery_items_live_idx ON grocery_items (list_id)          WHERE deleted_at IS NULL;
