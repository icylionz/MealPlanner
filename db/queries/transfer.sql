-- Full-fidelity export/import queries (FR15), scoped to one household. Exports
-- read the active household's entities ordered deterministically; imports upsert
-- by stable UUID into the active household so a re-import merges rather than
-- duplicates. Each upsert returns whether the row was newly inserted (xmax = 0)
-- so the import can report inserted vs updated counts. Membership/accounts are
-- not part of an archive — they are managed via invites, not import.

-- name: ExportFoods :many
SELECT * FROM foods WHERE household_id = $1 ORDER BY created_at, id;

-- name: ExportFoodTags :many
SELECT ft.food_id, ft.tag FROM food_tags ft
JOIN foods f ON f.id = ft.food_id
WHERE f.household_id = $1
ORDER BY ft.food_id, ft.tag;

-- name: ExportFoodAliases :many
SELECT fa.* FROM food_aliases fa
JOIN foods f ON f.id = fa.food_id
WHERE f.household_id = $1
ORDER BY fa.food_id, lower(fa.alias), fa.id;

-- name: ExportFoodComponents :many
SELECT fc.* FROM food_components fc
JOIN foods f ON f.id = fc.parent_food_id
WHERE f.household_id = $1
ORDER BY fc.parent_food_id, fc.sort_order, fc.id;

-- name: ExportFoodSteps :many
SELECT fs.* FROM food_steps fs
JOIN foods f ON f.id = fs.food_id
WHERE f.household_id = $1
ORDER BY fs.food_id, fs.step_number;

-- name: ExportMealSeries :many
SELECT * FROM meal_series WHERE household_id = $1 ORDER BY start_date, id;

-- name: ExportMealPlans :many
SELECT * FROM meal_plan WHERE household_id = $1 ORDER BY plan_date, plan_time, id;

-- name: ExportScheduledMealRecipes :many
SELECT smr.* FROM scheduled_meal_recipes smr
JOIN meal_plan mp ON mp.id = smr.meal_id
WHERE mp.household_id = $1
ORDER BY smr.meal_id, smr.sort_order, smr.id;

-- name: ExportGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 ORDER BY created_at, id;

-- name: ExportGroceryItems :many
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1
ORDER BY gi.list_id, gi.sort_order, gi.id;

-- name: ExportGroceryItemSources :many
SELECT gis.* FROM grocery_item_sources gis
JOIN grocery_items gi ON gi.id = gis.grocery_item_id
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1
ORDER BY gis.grocery_item_id, gis.id;

-- name: ExportPrepSessions :many
SELECT * FROM prep_sessions WHERE household_id = $1 ORDER BY session_date, id;

-- name: ExportPrepSessionMeals :many
SELECT psm.* FROM prep_session_meals psm
JOIN prep_sessions ps ON ps.id = psm.session_id
WHERE ps.household_id = $1
ORDER BY psm.session_id, psm.sort_order;

-- Which UUIDs already exist in this household, so an import can resolve foreign
-- keys without tripping constraints for sections the caller chose not to import.
-- name: ExistingFoodIDs :many
SELECT id FROM foods WHERE household_id = $1;

-- name: ExistingMealSeriesIDs :many
SELECT id FROM meal_series WHERE household_id = $1;

-- name: ExistingMealPlanIDs :many
SELECT id FROM meal_plan WHERE household_id = $1;

-- name: ExistingFoodComponentIDs :many
SELECT fc.id FROM food_components fc
JOIN foods f ON f.id = fc.parent_food_id
WHERE f.household_id = $1;

-- Authorship may only be restored for accounts that belong to the destination
-- household. Archives intentionally do not carry accounts or memberships.
-- name: ExistingHouseholdAccountIDs :many
SELECT account_id FROM household_members WHERE household_id = $1;

-- Preflight every stable UUID namespace. Child rows derive ownership through
-- their parent, so a collision can be reported before any writes are attempted.
-- name: TransferIDOwners :many
WITH requested(id) AS (SELECT unnest($1::uuid[]))
SELECT 'food'::text AS entity_type, f.id, f.household_id
FROM foods f JOIN requested r ON r.id = f.id
UNION ALL
SELECT 'food_alias'::text, fa.id, f.household_id
FROM food_aliases fa JOIN foods f ON f.id = fa.food_id JOIN requested r ON r.id = fa.id
UNION ALL
SELECT 'food_component'::text, fc.id, f.household_id
FROM food_components fc JOIN foods f ON f.id = fc.parent_food_id JOIN requested r ON r.id = fc.id
UNION ALL
SELECT 'meal_series'::text, ms.id, ms.household_id
FROM meal_series ms JOIN requested r ON r.id = ms.id
UNION ALL
SELECT 'meal_plan'::text, mp.id, mp.household_id
FROM meal_plan mp JOIN requested r ON r.id = mp.id
UNION ALL
SELECT 'scheduled_meal_recipe'::text, smr.id, mp.household_id
FROM scheduled_meal_recipes smr
JOIN meal_plan mp ON mp.id = smr.meal_id
JOIN requested r ON r.id = smr.id
UNION ALL
SELECT 'grocery_list'::text, gl.id, gl.household_id
FROM grocery_lists gl JOIN requested r ON r.id = gl.id
UNION ALL
SELECT 'grocery_item'::text, gi.id, gl.household_id
FROM grocery_items gi JOIN grocery_lists gl ON gl.id = gi.list_id JOIN requested r ON r.id = gi.id
UNION ALL
SELECT 'grocery_item_source'::text, gis.id, gl.household_id
FROM grocery_item_sources gis
JOIN grocery_items gi ON gi.id = gis.grocery_item_id
JOIN grocery_lists gl ON gl.id = gi.list_id
JOIN requested r ON r.id = gis.id
UNION ALL
SELECT 'prep_session'::text, ps.id, ps.household_id
FROM prep_sessions ps JOIN requested r ON r.id = ps.id;

-- Version 1 upserts deliberately omit fields introduced after the v1 archive
-- contract. Updating an old archive must not clear newer local metadata.
-- name: ImportFoodV1 :one
INSERT INTO foods (id, household_id, name, description, prep_time_min, cook_time_min, servings,
    default_unit, created_at, updated_at, density_g_per_ml, density_source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, description = excluded.description,
    prep_time_min = excluded.prep_time_min, cook_time_min = excluded.cook_time_min,
    servings = excluded.servings, default_unit = excluded.default_unit,
    updated_at = excluded.updated_at,
    density_g_per_ml = excluded.density_g_per_ml, density_source = excluded.density_source
WHERE foods.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodV2 :one
INSERT INTO foods (id, household_id, name, description, prep_time_min, cook_time_min, servings,
    default_unit, created_at, updated_at, density_g_per_ml, density_source,
    source_url, source_last_imported_at, deleted_at, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, description = excluded.description,
    prep_time_min = excluded.prep_time_min, cook_time_min = excluded.cook_time_min,
    servings = excluded.servings, default_unit = excluded.default_unit,
    updated_at = excluded.updated_at,
    density_g_per_ml = excluded.density_g_per_ml, density_source = excluded.density_source,
    source_url = excluded.source_url, source_last_imported_at = excluded.source_last_imported_at,
    deleted_at = excluded.deleted_at, created_by = excluded.created_by, updated_by = excluded.updated_by
WHERE foods.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2)
ON CONFLICT (food_id, tag) DO NOTHING;

-- name: ImportFoodAlias :one
INSERT INTO food_aliases (id, food_id, alias, created_at, created_by)
SELECT $1, $2, $3, $4, $5
FROM foods parent
WHERE parent.id = $2 AND parent.household_id = $6
ON CONFLICT (id) DO UPDATE SET
    food_id = excluded.food_id, alias = excluded.alias,
    created_at = excluded.created_at, created_by = excluded.created_by
WHERE EXISTS (
    SELECT 1 FROM foods current_parent
    WHERE current_parent.id = food_aliases.food_id AND current_parent.household_id = $6
)
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodComponentV1 :one
INSERT INTO food_components (id, parent_food_id, child_food_id, amount, unit, sort_order)
SELECT $1, $2, $3, $4, $5, $6
WHERE EXISTS (SELECT 1 FROM foods f WHERE f.id = $2 AND f.household_id = $7)
  AND EXISTS (SELECT 1 FROM foods f WHERE f.id = $3 AND f.household_id = $7)
ON CONFLICT (id) DO UPDATE SET
    parent_food_id = excluded.parent_food_id, child_food_id = excluded.child_food_id,
    amount = excluded.amount, unit = excluded.unit, sort_order = excluded.sort_order
WHERE EXISTS (
    SELECT 1 FROM foods current_parent
    WHERE current_parent.id = food_components.parent_food_id AND current_parent.household_id = $7
)
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodComponentV2 :one
INSERT INTO food_components (id, parent_food_id, child_food_id, amount, unit, variant_text, sort_order)
SELECT $1, $2, $3, $4, $5, $6, $7
WHERE EXISTS (SELECT 1 FROM foods f WHERE f.id = $2 AND f.household_id = $8)
  AND EXISTS (SELECT 1 FROM foods f WHERE f.id = $3 AND f.household_id = $8)
ON CONFLICT (id) DO UPDATE SET
    parent_food_id = excluded.parent_food_id, child_food_id = excluded.child_food_id,
    amount = excluded.amount, unit = excluded.unit,
    variant_text = excluded.variant_text, sort_order = excluded.sort_order
WHERE EXISTS (
    SELECT 1 FROM foods current_parent
    WHERE current_parent.id = food_components.parent_food_id AND current_parent.household_id = $8
)
RETURNING (xmax = 0) AS inserted;

-- name: DeleteImportedFoodComponentsExcept :exec
DELETE FROM food_components fc
USING foods parent
WHERE fc.parent_food_id = $1 AND parent.id = fc.parent_food_id
  AND parent.household_id = $2 AND NOT (fc.id = ANY(sqlc.arg(keep_ids)::uuid[]));

-- name: ImportFoodStep :exec
INSERT INTO food_steps (food_id, step_number, instruction)
VALUES ($1, $2, $3)
ON CONFLICT (food_id, step_number) DO UPDATE SET instruction = excluded.instruction;

-- name: ImportMealSeries :one
INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date, version)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
WHERE EXISTS (SELECT 1 FROM foods f WHERE f.id = $3 AND f.household_id = $2)
ON CONFLICT (id) DO UPDATE SET
    food_id = excluded.food_id, plan_time = excluded.plan_time,
    servings = excluded.servings, freq = excluded.freq, byweekday = excluded.byweekday,
    start_date = excluded.start_date, until_date = excluded.until_date,
    version = meal_series.version + 1
WHERE meal_series.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportMealPlanV1 :one
INSERT INTO meal_plan (id, household_id, plan_date, plan_time, food_id, servings, series_id,
    link_url, link_title, link_image_url)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
WHERE EXISTS (SELECT 1 FROM foods f WHERE f.id = $5 AND f.household_id = $2)
  AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM meal_series ms WHERE ms.id = $7 AND ms.household_id = $2))
ON CONFLICT (id) DO UPDATE SET
    plan_date = excluded.plan_date, plan_time = excluded.plan_time,
    food_id = excluded.food_id, servings = excluded.servings, series_id = excluded.series_id,
    link_url = excluded.link_url, link_title = excluded.link_title,
    link_image_url = excluded.link_image_url, version = meal_plan.version + 1
WHERE meal_plan.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: GetImportedMealSeries :one
SELECT series_id
FROM meal_plan
WHERE id = sqlc.arg(id) AND household_id = sqlc.arg(household_id);

-- name: ImportMealPlanV2 :one
INSERT INTO meal_plan (id, household_id, plan_date, plan_time, food_id, servings, series_id,
    link_url, link_title, link_image_url, title, notes, version, deleted_at, created_by, updated_by)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
WHERE EXISTS (SELECT 1 FROM foods f WHERE f.id = $5 AND f.household_id = $2)
  AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM meal_series ms WHERE ms.id = $7 AND ms.household_id = $2))
ON CONFLICT (id) DO UPDATE SET
    plan_date = excluded.plan_date, plan_time = excluded.plan_time,
    food_id = excluded.food_id, servings = excluded.servings, series_id = excluded.series_id,
    link_url = excluded.link_url, link_title = excluded.link_title,
    link_image_url = excluded.link_image_url, title = excluded.title, notes = excluded.notes,
    version = meal_plan.version + 1, deleted_at = excluded.deleted_at,
    created_by = excluded.created_by, updated_by = excluded.updated_by
WHERE meal_plan.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: BumpImportedMealSeriesVersion :execrows
-- Selective imports may update one occurrence without carrying its series row.
-- Invalidate every editor opened against that attached aggregate.
UPDATE meal_series
SET version = version + 1
WHERE id = sqlc.arg(id) AND household_id = sqlc.arg(household_id);

-- name: DeleteImportedScheduledMealRecipes :exec
DELETE FROM scheduled_meal_recipes smr
USING meal_plan mp
WHERE smr.meal_id = $1 AND mp.id = smr.meal_id AND mp.household_id = $2;

-- name: ImportScheduledMealRecipe :one
INSERT INTO scheduled_meal_recipes
    (id, meal_id, food_id, servings_override, sort_order, created_by, updated_by)
SELECT $1, $2, $3, $4, $5, $6, $7
WHERE EXISTS (SELECT 1 FROM meal_plan mp WHERE mp.id = $2 AND mp.household_id = $8)
  AND EXISTS (SELECT 1 FROM foods f WHERE f.id = $3 AND f.household_id = $8)
ON CONFLICT (id) DO UPDATE SET
    meal_id = excluded.meal_id, food_id = excluded.food_id,
    servings_override = excluded.servings_override, sort_order = excluded.sort_order,
    created_by = excluded.created_by, updated_by = excluded.updated_by
WHERE EXISTS (
    SELECT 1 FROM meal_plan current_parent
    WHERE current_parent.id = scheduled_meal_recipes.meal_id AND current_parent.household_id = $8
)
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryListV1 :one
INSERT INTO grocery_lists (id, household_id, name, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = excluded.name
WHERE grocery_lists.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryListV2 :one
INSERT INTO grocery_lists (id, household_id, name, created_at, deleted_at, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, deleted_at = excluded.deleted_at,
    created_by = excluded.created_by, updated_by = excluded.updated_by
WHERE grocery_lists.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryItemV1 :one
INSERT INTO grocery_items (id, list_id, name, amount, unit, checked, note, sort_order)
SELECT $1, $2, $3, $4, $5, $6, $7, $8
FROM grocery_lists parent
WHERE parent.id = $2 AND parent.household_id = $9
ON CONFLICT (id) DO UPDATE SET
    list_id = excluded.list_id, name = excluded.name, amount = excluded.amount,
    unit = excluded.unit, checked = excluded.checked, note = excluded.note,
    sort_order = excluded.sort_order
WHERE EXISTS (
    SELECT 1 FROM grocery_lists current_parent
    WHERE current_parent.id = grocery_items.list_id AND current_parent.household_id = $9
)
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryItemV2 :one
INSERT INTO grocery_items (id, list_id, name, amount, unit, checked, note, sort_order,
    ingredient_id, source_type, variant_text, deleted_at, created_by, updated_by)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
FROM grocery_lists parent
WHERE parent.id = $2 AND parent.household_id = $15
  AND ($9::uuid IS NULL OR EXISTS (SELECT 1 FROM foods WHERE id = $9 AND household_id = $15))
ON CONFLICT (id) DO UPDATE SET
    list_id = excluded.list_id, name = excluded.name, amount = excluded.amount,
    unit = excluded.unit, checked = excluded.checked, note = excluded.note,
    sort_order = excluded.sort_order, ingredient_id = excluded.ingredient_id,
    source_type = excluded.source_type, variant_text = excluded.variant_text,
    deleted_at = excluded.deleted_at, created_by = excluded.created_by, updated_by = excluded.updated_by
WHERE EXISTS (
    SELECT 1 FROM grocery_lists current_parent
    WHERE current_parent.id = grocery_items.list_id AND current_parent.household_id = $15
)
RETURNING (xmax = 0) AS inserted;

-- name: DeleteImportedGroceryItemSources :exec
DELETE FROM grocery_item_sources gis
USING grocery_items gi, grocery_lists gl
WHERE gis.grocery_item_id = $1 AND gi.id = gis.grocery_item_id
  AND gl.id = gi.list_id AND gl.household_id = $2;

-- name: ImportGroceryItemSource :one
INSERT INTO grocery_item_sources (id, grocery_item_id, scheduled_meal_id, recipe_id,
    recipe_ingredient_line_id, quantity_contributed, unit_contributed,
    variant_text, line_recipe_name)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
WHERE EXISTS (
    SELECT 1 FROM grocery_items gi JOIN grocery_lists gl ON gl.id = gi.list_id
    WHERE gi.id = $2 AND gl.household_id = $10
)
  AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM meal_plan WHERE id = $3 AND household_id = $10))
  AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM foods WHERE id = $4 AND household_id = $10))
  AND ($5::uuid IS NULL OR EXISTS (
      SELECT 1 FROM food_components fc JOIN foods f ON f.id = fc.parent_food_id
      WHERE fc.id = $5 AND f.household_id = $10
  ))
ON CONFLICT (id) DO UPDATE SET
    grocery_item_id = excluded.grocery_item_id,
    scheduled_meal_id = excluded.scheduled_meal_id,
    recipe_id = excluded.recipe_id,
    recipe_ingredient_line_id = excluded.recipe_ingredient_line_id,
    quantity_contributed = excluded.quantity_contributed,
    unit_contributed = excluded.unit_contributed,
    variant_text = excluded.variant_text,
    line_recipe_name = excluded.line_recipe_name
WHERE EXISTS (
    SELECT 1 FROM grocery_items gi JOIN grocery_lists gl ON gl.id = gi.list_id
    WHERE gi.id = grocery_item_sources.grocery_item_id AND gl.household_id = $10
)
RETURNING (xmax = 0) AS inserted;

-- name: ImportPrepSession :one
INSERT INTO prep_sessions (id, household_id, name, session_date, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET name = excluded.name, session_date = excluded.session_date
WHERE prep_sessions.household_id = excluded.household_id
RETURNING (xmax = 0) AS inserted;

-- name: ImportPrepSessionMeal :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
SELECT $1, $2, $3, $4
WHERE EXISTS (SELECT 1 FROM prep_sessions ps WHERE ps.id = $1 AND ps.household_id = $5)
  AND EXISTS (SELECT 1 FROM foods f WHERE f.id = $2 AND f.household_id = $5)
ON CONFLICT (session_id, food_id) DO UPDATE SET
    servings = excluded.servings, sort_order = excluded.sort_order;
