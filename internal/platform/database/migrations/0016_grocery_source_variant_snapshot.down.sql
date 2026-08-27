ALTER TABLE grocery_item_sources
    DROP COLUMN line_recipe_name,
    DROP COLUMN variant_text;

ALTER TABLE grocery_items
    DROP COLUMN variant_text;
