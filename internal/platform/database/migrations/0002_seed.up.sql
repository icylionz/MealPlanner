-- Seed data mirroring the Backbone Plate prototype sample state.
-- Meal plan dates are laid out over the week containing the migration run.

INSERT INTO household_members (id, name, role, initials, color) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'Alex Morgan', 'owner',  'AM', '#22386A'),
    ('a0000000-0000-0000-0000-000000000002', 'Sam Morgan',  'member', 'SM', '#1E8E5A');

INSERT INTO recipes (id, name, description, prep_time_min, cook_time_min, servings) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'Sourdough starter', 'Live culture for leavening bread. Takes about a week to get going.', 10, 0, 1),
    ('b0000000-0000-0000-0000-000000000002', 'Sourdough bread', 'Open crumb, chewy crust, tangy flavour.', 30, 50, 8),
    ('b0000000-0000-0000-0000-000000000003', 'Tomato sauce', 'Simple crushed tomato sauce. Makes enough for 2–3 pizzas.', 5, 25, 4),
    ('b0000000-0000-0000-0000-000000000004', 'Pizza dough', 'Thin, crisp Neapolitan-style dough. Proof overnight for best flavour.', 15, 0, 2),
    ('b0000000-0000-0000-0000-000000000005', 'Margherita pizza', 'Classic margherita — sauce, mozzarella, basil.', 20, 12, 2),
    ('b0000000-0000-0000-0000-000000000006', 'Oat porridge', 'Simple, warm, filling breakfast.', 2, 8, 1),
    ('b0000000-0000-0000-0000-000000000007', 'Grilled chicken', 'Juicy, simply seasoned chicken breast.', 10, 15, 2),
    ('b0000000-0000-0000-0000-000000000008', 'Caesar salad', 'Classic caesar with grilled chicken.', 15, 0, 2);

INSERT INTO recipe_tags (recipe_id, tag) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'base'),
    ('b0000000-0000-0000-0000-000000000001', 'bread'),
    ('b0000000-0000-0000-0000-000000000002', 'bread'),
    ('b0000000-0000-0000-0000-000000000002', 'bake'),
    ('b0000000-0000-0000-0000-000000000003', 'sauce'),
    ('b0000000-0000-0000-0000-000000000003', 'base'),
    ('b0000000-0000-0000-0000-000000000003', 'italian'),
    ('b0000000-0000-0000-0000-000000000004', 'dough'),
    ('b0000000-0000-0000-0000-000000000004', 'base'),
    ('b0000000-0000-0000-0000-000000000004', 'italian'),
    ('b0000000-0000-0000-0000-000000000005', 'dinner'),
    ('b0000000-0000-0000-0000-000000000005', 'italian'),
    ('b0000000-0000-0000-0000-000000000006', 'breakfast'),
    ('b0000000-0000-0000-0000-000000000007', 'protein'),
    ('b0000000-0000-0000-0000-000000000007', 'dinner'),
    ('b0000000-0000-0000-0000-000000000007', 'base'),
    ('b0000000-0000-0000-0000-000000000008', 'lunch'),
    ('b0000000-0000-0000-0000-000000000008', 'salad');

INSERT INTO recipe_ingredients (recipe_id, name, amount, unit, sub_recipe_id, sort_order) VALUES
    -- Sourdough starter
    ('b0000000-0000-0000-0000-000000000001', 'Bread flour', 50, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000001', 'Water', 50, 'ml', NULL, 1),
    -- Sourdough bread
    ('b0000000-0000-0000-0000-000000000002', 'Bread flour', 500, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000002', 'Water', 350, 'ml', NULL, 1),
    ('b0000000-0000-0000-0000-000000000002', 'Salt', 10, 'g', NULL, 2),
    ('b0000000-0000-0000-0000-000000000002', '', 100, 'g', 'b0000000-0000-0000-0000-000000000001', 3),
    -- Tomato sauce
    ('b0000000-0000-0000-0000-000000000003', 'Canned tomatoes', 400, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000003', 'Garlic', 3, 'count', NULL, 1),
    ('b0000000-0000-0000-0000-000000000003', 'Olive oil', 30, 'ml', NULL, 2),
    ('b0000000-0000-0000-0000-000000000003', 'Fresh basil', 8, 'g', NULL, 3),
    ('b0000000-0000-0000-0000-000000000003', 'Salt', 4, 'g', NULL, 4),
    -- Pizza dough
    ('b0000000-0000-0000-0000-000000000004', 'Bread flour', 300, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000004', 'Water', 185, 'ml', NULL, 1),
    ('b0000000-0000-0000-0000-000000000004', 'Instant yeast', 4, 'g', NULL, 2),
    ('b0000000-0000-0000-0000-000000000004', 'Salt', 6, 'g', NULL, 3),
    ('b0000000-0000-0000-0000-000000000004', 'Olive oil', 15, 'ml', NULL, 4),
    -- Margherita pizza
    ('b0000000-0000-0000-0000-000000000005', '', 1, 'count', 'b0000000-0000-0000-0000-000000000004', 0),
    ('b0000000-0000-0000-0000-000000000005', '', 2, 'count', 'b0000000-0000-0000-0000-000000000003', 1),
    ('b0000000-0000-0000-0000-000000000005', 'Mozzarella', 150, 'g', NULL, 2),
    ('b0000000-0000-0000-0000-000000000005', 'Fresh basil', 5, 'g', NULL, 3),
    -- Oat porridge
    ('b0000000-0000-0000-0000-000000000006', 'Rolled oats', 80, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000006', 'Oat milk', 250, 'ml', NULL, 1),
    ('b0000000-0000-0000-0000-000000000006', 'Honey', 15, 'ml', NULL, 2),
    ('b0000000-0000-0000-0000-000000000006', 'Banana', 1, 'count', NULL, 3),
    ('b0000000-0000-0000-0000-000000000006', 'Ground cinnamon', 1, 'g', NULL, 4),
    -- Grilled chicken
    ('b0000000-0000-0000-0000-000000000007', 'Chicken breast', 400, 'g', NULL, 0),
    ('b0000000-0000-0000-0000-000000000007', 'Olive oil', 20, 'ml', NULL, 1),
    ('b0000000-0000-0000-0000-000000000007', 'Lemon juice', 30, 'ml', NULL, 2),
    ('b0000000-0000-0000-0000-000000000007', 'Garlic', 2, 'count', NULL, 3),
    ('b0000000-0000-0000-0000-000000000007', 'Salt', 5, 'g', NULL, 4),
    ('b0000000-0000-0000-0000-000000000007', 'Black pepper', 2, 'g', NULL, 5),
    -- Caesar salad
    ('b0000000-0000-0000-0000-000000000008', '', 1, 'count', 'b0000000-0000-0000-0000-000000000007', 0),
    ('b0000000-0000-0000-0000-000000000008', 'Romaine lettuce', 200, 'g', NULL, 1),
    ('b0000000-0000-0000-0000-000000000008', 'Parmesan', 40, 'g', NULL, 2),
    ('b0000000-0000-0000-0000-000000000008', 'Croutons', 60, 'g', NULL, 3),
    ('b0000000-0000-0000-0000-000000000008', 'Caesar dressing', 60, 'ml', NULL, 4);

INSERT INTO recipe_steps (recipe_id, step_number, instruction) VALUES
    ('b0000000-0000-0000-0000-000000000001', 1, 'Mix flour and water in a clean jar.'),
    ('b0000000-0000-0000-0000-000000000001', 2, 'Cover loosely. Leave at room temperature (20–24°C) for 24h.'),
    ('b0000000-0000-0000-0000-000000000001', 3, 'Discard half, feed again with 50g flour + 50g water. Repeat daily for 5–7 days until consistently bubbly.'),
    ('b0000000-0000-0000-0000-000000000002', 1, 'Mix flour and 325ml water. Rest 30 min (autolyse).'),
    ('b0000000-0000-0000-0000-000000000002', 2, 'Add starter and salt. Mix well.'),
    ('b0000000-0000-0000-0000-000000000002', 3, 'Fold every 30 min for 3–4 hours.'),
    ('b0000000-0000-0000-0000-000000000002', 4, 'Shape and proof overnight in the fridge.'),
    ('b0000000-0000-0000-0000-000000000002', 5, 'Bake at 250°C covered for 20 min, uncovered for 30 min.'),
    ('b0000000-0000-0000-0000-000000000003', 1, 'Heat olive oil. Sauté crushed garlic until fragrant, about 2 min.'),
    ('b0000000-0000-0000-0000-000000000003', 2, 'Add tomatoes. Crush with a spoon.'),
    ('b0000000-0000-0000-0000-000000000003', 3, 'Simmer uncovered 20 min. Season. Tear basil in off heat.'),
    ('b0000000-0000-0000-0000-000000000004', 1, 'Mix all ingredients. Knead 10 min until smooth.'),
    ('b0000000-0000-0000-0000-000000000004', 2, 'Divide into 2 balls. Cover and rest 1h at room temp.'),
    ('b0000000-0000-0000-0000-000000000004', 3, 'Refrigerate overnight (up to 3 days). Come to room temp 1h before use.'),
    ('b0000000-0000-0000-0000-000000000005', 1, 'Preheat oven to max (250°C+) with a baking stone inside.'),
    ('b0000000-0000-0000-0000-000000000005', 2, 'Stretch dough into a ~30cm round.'),
    ('b0000000-0000-0000-0000-000000000005', 3, 'Spread sauce thinly. Tear mozzarella over the top.'),
    ('b0000000-0000-0000-0000-000000000005', 4, 'Bake 10–12 min until crust is blistered. Add basil after baking.'),
    ('b0000000-0000-0000-0000-000000000006', 1, 'Bring oat milk to a gentle simmer.'),
    ('b0000000-0000-0000-0000-000000000006', 2, 'Add oats. Cook 5 min, stirring often, until thick.'),
    ('b0000000-0000-0000-0000-000000000006', 3, 'Serve topped with sliced banana, honey, and cinnamon.'),
    ('b0000000-0000-0000-0000-000000000007', 1, 'Marinate chicken in oil, lemon, garlic, salt, pepper for at least 30 min.'),
    ('b0000000-0000-0000-0000-000000000007', 2, 'Grill over high heat, 6–7 min per side.'),
    ('b0000000-0000-0000-0000-000000000007', 3, 'Rest 5 min before slicing.'),
    ('b0000000-0000-0000-0000-000000000008', 1, 'Tear lettuce into a large bowl.'),
    ('b0000000-0000-0000-0000-000000000008', 2, 'Slice grilled chicken and lay on top.'),
    ('b0000000-0000-0000-0000-000000000008', 3, 'Add croutons and parmesan shavings.'),
    ('b0000000-0000-0000-0000-000000000008', 4, 'Drizzle with caesar dressing. Toss gently.');

-- Week layout: Monday of the current ISO week + offset days.
INSERT INTO meal_plan (plan_date, plan_time, recipe_id, servings) VALUES
    (date_trunc('week', current_date)::date + 0, '08:00', 'b0000000-0000-0000-0000-000000000006', 1),
    (date_trunc('week', current_date)::date + 0, '12:30', 'b0000000-0000-0000-0000-000000000008', 2),
    (date_trunc('week', current_date)::date + 0, '19:00', 'b0000000-0000-0000-0000-000000000005', 2),
    (date_trunc('week', current_date)::date + 1, '07:45', 'b0000000-0000-0000-0000-000000000006', 1),
    (date_trunc('week', current_date)::date + 1, '19:30', 'b0000000-0000-0000-0000-000000000007', 2),
    (date_trunc('week', current_date)::date + 2, '08:00', 'b0000000-0000-0000-0000-000000000006', 1),
    (date_trunc('week', current_date)::date + 2, '12:00', 'b0000000-0000-0000-0000-000000000008', 2),
    (date_trunc('week', current_date)::date + 2, '19:00', 'b0000000-0000-0000-0000-000000000005', 2),
    (date_trunc('week', current_date)::date + 3, '08:00', 'b0000000-0000-0000-0000-000000000006', 1),
    (date_trunc('week', current_date)::date + 3, '19:30', 'b0000000-0000-0000-0000-000000000007', 2),
    (date_trunc('week', current_date)::date + 4, '07:30', 'b0000000-0000-0000-0000-000000000006', 1),
    (date_trunc('week', current_date)::date + 4, '12:30', 'b0000000-0000-0000-0000-000000000008', 2),
    (date_trunc('week', current_date)::date + 4, '19:00', 'b0000000-0000-0000-0000-000000000005', 2),
    (date_trunc('week', current_date)::date + 5, '09:00', 'b0000000-0000-0000-0000-000000000006', 2),
    (date_trunc('week', current_date)::date + 5, '13:00', 'b0000000-0000-0000-0000-000000000008', 2),
    (date_trunc('week', current_date)::date + 6, '09:30', 'b0000000-0000-0000-0000-000000000006', 2),
    (date_trunc('week', current_date)::date + 6, '11:00', 'b0000000-0000-0000-0000-000000000002', 4);

INSERT INTO grocery_lists (id, name) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'Weekly shop');

INSERT INTO grocery_items (list_id, name, amount, unit, checked, sort_order) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'Rolled oats', 560, 'g', false, 0),
    ('c0000000-0000-0000-0000-000000000001', 'Oat milk', 1750, 'ml', false, 1),
    ('c0000000-0000-0000-0000-000000000001', 'Banana', 7, 'count', true, 2),
    ('c0000000-0000-0000-0000-000000000001', 'Honey', 105, 'ml', false, 3),
    ('c0000000-0000-0000-0000-000000000001', 'Ground cinnamon', 7, 'g', false, 4),
    ('c0000000-0000-0000-0000-000000000001', 'Bread flour', 600, 'g', false, 5),
    ('c0000000-0000-0000-0000-000000000001', 'Instant yeast', 8, 'g', true, 6),
    ('c0000000-0000-0000-0000-000000000001', 'Mozzarella', 300, 'g', false, 7),
    ('c0000000-0000-0000-0000-000000000001', 'Canned tomatoes', 800, 'g', true, 8),
    ('c0000000-0000-0000-0000-000000000001', 'Garlic', 9, 'count', false, 9),
    ('c0000000-0000-0000-0000-000000000001', 'Olive oil', 135, 'ml', false, 10),
    ('c0000000-0000-0000-0000-000000000001', 'Chicken breast', 800, 'g', false, 11),
    ('c0000000-0000-0000-0000-000000000001', 'Romaine lettuce', 600, 'g', false, 12),
    ('c0000000-0000-0000-0000-000000000001', 'Parmesan', 120, 'g', false, 13),
    ('c0000000-0000-0000-0000-000000000001', 'Croutons', 180, 'g', false, 14);
