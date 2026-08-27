-- Household teardown reaches grocery history through both CASCADE and SET NULL
-- paths. Defer the intersecting checks so PostgreSQL can finish all referential
-- actions before validating the final transaction state.
ALTER TABLE food_components
    DROP CONSTRAINT food_components_child_food_id_fkey,
    ADD CONSTRAINT food_components_child_food_id_fkey
        FOREIGN KEY (child_food_id) REFERENCES foods (id)
        ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE grocery_items
    ALTER CONSTRAINT grocery_items_list_id_fkey DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT grocery_items_ingredient_id_fkey DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE grocery_item_sources
    ALTER CONSTRAINT grocery_item_sources_grocery_item_id_fkey DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT grocery_item_sources_scheduled_meal_id_fkey DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT grocery_item_sources_recipe_id_fkey DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT grocery_item_sources_recipe_ingredient_line_id_fkey DEFERRABLE INITIALLY DEFERRED;
