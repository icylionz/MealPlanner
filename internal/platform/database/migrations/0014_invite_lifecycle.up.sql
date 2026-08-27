CREATE TABLE invites (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    code         text NOT NULL,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    max_uses     integer CHECK (max_uses IS NULL OR max_uses > 0),
    use_count    integer NOT NULL DEFAULT 0 CHECK (use_count >= 0),
    created_by   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (max_uses IS NULL OR use_count <= max_uses)
);

CREATE UNIQUE INDEX invites_code_idx ON invites (upper(code));
CREATE UNIQUE INDEX invites_active_household_idx
    ON invites (household_id) WHERE revoked_at IS NULL;
CREATE INDEX invites_household_created_idx ON invites (household_id, created_at DESC);

-- Preserve each existing share code as a time-limited lifecycle invite. The
-- template household is not joinable and therefore needs no invite.
INSERT INTO invites (household_id, code, expires_at, created_by)
SELECT h.id,
       h.invite_code,
       now() + interval '7 days',
       (
           SELECT m.account_id
           FROM household_members m
           WHERE m.household_id = h.id
           ORDER BY (m.role = 'owner') DESC, m.created_at
           LIMIT 1
       )
FROM households h
WHERE h.is_template = false;

DROP INDEX households_invite_code_idx;
ALTER TABLE households DROP COLUMN invite_code;
