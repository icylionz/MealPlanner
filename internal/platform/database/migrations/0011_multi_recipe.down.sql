DROP TABLE IF EXISTS scheduled_meal_recipes;

ALTER TABLE meal_plan
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS version;
