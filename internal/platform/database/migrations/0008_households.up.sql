-- Multi-tenant households. An account is the login identity; a household owns
-- its own foods, meal plans, grocery lists and prep sessions. An account joins
-- one or more households via household_members, and its session tracks which
-- household is currently active.

-- 1. Accounts: the login identity (was household_members.email/password_hash).
CREATE TABLE accounts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,
    name          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX accounts_email_idx ON accounts (lower(email));

-- 2. Households, joinable by a shareable invite code. is_template marks the
-- seeded starter household that new households are cloned from; it has no
-- members and is never shown.
CREATE TABLE households (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    invite_code text NOT NULL,
    is_template boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX households_invite_code_idx ON households (invite_code);

-- 3. household_members becomes the account<->household join. Drop the old
-- profile table (its email/password moved to accounts) and recreate it. sessions
-- FKs household_members, so drop it first; both are recreated below. The
-- fresh-install seed no longer creates members, so nothing of value is lost.
DROP TABLE sessions;
DROP TABLE household_members;
CREATE TABLE household_members (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    account_id   uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    role         text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    initials     text NOT NULL,
    color        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, account_id)
);
CREATE INDEX household_members_account_idx ON household_members (account_id);

-- 4. Sessions now key off the account and carry the active household.
CREATE TABLE sessions (
    token               text PRIMARY KEY,
    account_id          uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    active_household_id uuid REFERENCES households (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL
);

-- 5. Scope the domain tables by household. Seeded foods (loaded by 0002 with no
-- household) are assigned to a template household created here; new households
-- clone that template. Only top-level tables carry household_id — children scope
-- transitively through their parent (food_tags/components/steps via food_id,
-- grocery_items via list_id, prep_session_meals via session_id, meal_plan via
-- series_id).
ALTER TABLE foods          ADD COLUMN household_id uuid REFERENCES households (id) ON DELETE CASCADE;
ALTER TABLE meal_series    ADD COLUMN household_id uuid REFERENCES households (id) ON DELETE CASCADE;
ALTER TABLE meal_plan      ADD COLUMN household_id uuid REFERENCES households (id) ON DELETE CASCADE;
ALTER TABLE grocery_lists  ADD COLUMN household_id uuid REFERENCES households (id) ON DELETE CASCADE;
ALTER TABLE prep_sessions  ADD COLUMN household_id uuid REFERENCES households (id) ON DELETE CASCADE;

-- Template household holding the seeded starter foods.
INSERT INTO households (id, name, invite_code, is_template)
VALUES ('00000000-0000-0000-0000-0000000000ff', 'Starter Template', '__template__', true);

UPDATE foods         SET household_id = '00000000-0000-0000-0000-0000000000ff' WHERE household_id IS NULL;
UPDATE meal_series   SET household_id = '00000000-0000-0000-0000-0000000000ff' WHERE household_id IS NULL;
UPDATE meal_plan     SET household_id = '00000000-0000-0000-0000-0000000000ff' WHERE household_id IS NULL;
UPDATE grocery_lists SET household_id = '00000000-0000-0000-0000-0000000000ff' WHERE household_id IS NULL;
UPDATE prep_sessions SET household_id = '00000000-0000-0000-0000-0000000000ff' WHERE household_id IS NULL;

ALTER TABLE foods          ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE meal_series    ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE meal_plan      ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE grocery_lists  ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE prep_sessions  ALTER COLUMN household_id SET NOT NULL;

CREATE INDEX foods_household_idx         ON foods (household_id);
CREATE INDEX meal_series_household_idx   ON meal_series (household_id);
CREATE INDEX meal_plan_household_idx     ON meal_plan (household_id, plan_date, plan_time);
CREATE INDEX grocery_lists_household_idx ON grocery_lists (household_id);
CREATE INDEX prep_sessions_household_idx ON prep_sessions (household_id);
