-- G2: authorship columns. created_by/updated_by record the account that created
-- or last modified a row (PRD §6). Nullable on purpose: seeded template foods
-- (loaded by 0002 with no account) and any pre-existing rows have no known
-- author. ON DELETE SET NULL preserves domain data when an account is removed —
-- contrast with household_id, which CASCADEs.
ALTER TABLE foods
    ADD COLUMN created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    ADD COLUMN updated_by uuid REFERENCES accounts (id) ON DELETE SET NULL;
ALTER TABLE meal_plan
    ADD COLUMN created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    ADD COLUMN updated_by uuid REFERENCES accounts (id) ON DELETE SET NULL;
ALTER TABLE grocery_lists
    ADD COLUMN created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    ADD COLUMN updated_by uuid REFERENCES accounts (id) ON DELETE SET NULL;
ALTER TABLE grocery_items
    ADD COLUMN created_by uuid REFERENCES accounts (id) ON DELETE SET NULL,
    ADD COLUMN updated_by uuid REFERENCES accounts (id) ON DELETE SET NULL;
