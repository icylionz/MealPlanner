-- Reverse 0008: drop household scoping and restore the single-tenant member
-- model (household_members as profiles + sessions.member_id), as of 0007.
ALTER TABLE foods          DROP COLUMN household_id;
ALTER TABLE meal_series    DROP COLUMN household_id;
ALTER TABLE meal_plan      DROP COLUMN household_id;
ALTER TABLE grocery_lists  DROP COLUMN household_id;
ALTER TABLE prep_sessions  DROP COLUMN household_id;

DROP TABLE sessions;
DROP TABLE household_members;
DROP TABLE households;
DROP TABLE accounts;

CREATE TABLE household_members (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    role          text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    initials      text NOT NULL,
    color         text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    email         text,
    password_hash text
);
CREATE UNIQUE INDEX household_members_email_idx
    ON household_members (lower(email)) WHERE email IS NOT NULL;

CREATE TABLE sessions (
    token      text PRIMARY KEY,
    member_id  uuid NOT NULL REFERENCES household_members (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
