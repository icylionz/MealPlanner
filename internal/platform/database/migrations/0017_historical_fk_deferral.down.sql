ALTER TABLE grocery_item_sources
    ALTER CONSTRAINT grocery_item_sources_recipe_ingredient_line_id_fkey NOT DEFERRABLE,
    ALTER CONSTRAINT grocery_item_sources_recipe_id_fkey NOT DEFERRABLE,
    ALTER CONSTRAINT grocery_item_sources_scheduled_meal_id_fkey NOT DEFERRABLE,
    ALTER CONSTRAINT grocery_item_sources_grocery_item_id_fkey NOT DEFERRABLE;

ALTER TABLE grocery_items
    ALTER CONSTRAINT grocery_items_ingredient_id_fkey NOT DEFERRABLE,
    ALTER CONSTRAINT grocery_items_list_id_fkey NOT DEFERRABLE;

ALTER TABLE food_components
    DROP CONSTRAINT food_components_child_food_id_fkey,
    ADD CONSTRAINT food_components_child_food_id_fkey
        FOREIGN KEY (child_food_id) REFERENCES foods (id) ON DELETE RESTRICT;
