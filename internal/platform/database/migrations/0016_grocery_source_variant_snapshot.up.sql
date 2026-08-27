ALTER TABLE grocery_items
    ADD COLUMN variant_text text NOT NULL DEFAULT '';

ALTER TABLE grocery_item_sources
    ADD COLUMN variant_text text NOT NULL DEFAULT '',
    ADD COLUMN line_recipe_name text NOT NULL DEFAULT '';

UPDATE grocery_item_sources gis
SET variant_text = fc.variant_text,
    line_recipe_name = CASE
        WHEN fc.parent_food_id IS DISTINCT FROM gis.recipe_id THEN parent.name
        ELSE ''
    END
FROM food_components fc
JOIN foods parent ON parent.id = fc.parent_food_id
WHERE fc.id = gis.recipe_ingredient_line_id;
