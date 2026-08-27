-- G4: multi-recipe scheduled meals (PRD FR5 AC3–AC5, ScheduledMeal +
-- ScheduledMealRecipe). A scheduled meal keeps its primary recipe on
-- meal_plan.food_id (recipe #1) and may attach additional recipes through
-- scheduled_meal_recipes, each with an optional per-recipe servings override.
-- title/notes describe the meal; version supports optimistic locking (G13).
ALTER TABLE meal_plan
    ADD COLUMN title   text NOT NULL DEFAULT '',
    ADD COLUMN notes   text NOT NULL DEFAULT '',
    ADD COLUMN version int  NOT NULL DEFAULT 1;

CREATE TABLE scheduled_meal_recipes (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    meal_id           uuid NOT NULL REFERENCES meal_plan (id) ON DELETE CASCADE,
    food_id           uuid NOT NULL REFERENCES foods (id) ON DELETE CASCADE,
    servings_override int,                       -- NULL => fall back to meal servings
    sort_order        int  NOT NULL DEFAULT 0,
    created_by        uuid REFERENCES accounts (id) ON DELETE SET NULL,
    updated_by        uuid REFERENCES accounts (id) ON DELETE SET NULL
);

CREATE INDEX scheduled_meal_recipes_meal_idx ON scheduled_meal_recipes (meal_id, sort_order);
