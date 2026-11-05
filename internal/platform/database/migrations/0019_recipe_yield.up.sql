ALTER TABLE foods
    ADD COLUMN yield_amount numeric NOT NULL DEFAULT 1,
    ADD CONSTRAINT foods_yield_amount_positive CHECK (yield_amount > 0);

-- Existing count-based recipes used servings as their effective output count.
-- Preserve that meaning, then correct the two seeded recipes measured by mass.
UPDATE foods
SET yield_amount = servings
WHERE EXISTS (
    SELECT 1 FROM food_components fc WHERE fc.parent_food_id = foods.id
);

UPDATE foods
SET yield_amount = CASE lower(name)
    WHEN 'sourdough starter' THEN 100 -- starter, g
    WHEN 'sourdough bread' THEN 960   -- finished loaf, g
    ELSE yield_amount
END
WHERE default_unit = 'g'
  AND lower(name) IN ('sourdough starter', 'sourdough bread');
