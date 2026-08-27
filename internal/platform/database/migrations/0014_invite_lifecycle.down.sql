ALTER TABLE households ADD COLUMN invite_code text;

UPDATE households h
SET invite_code = COALESCE(
    (
        SELECT i.code
        FROM invites i
        WHERE i.household_id = h.id
          AND i.revoked_at IS NULL
          AND i.expires_at > now()
          AND (i.max_uses IS NULL OR i.use_count < i.max_uses)
        ORDER BY i.created_at DESC
        LIMIT 1
    ),
    -- Legacy invite codes cannot represent lifecycle state. Mint a fresh,
    -- unguessable bearer value rather than reviving an unusable code.
    'RESTORED-' || replace(gen_random_uuid()::text, '-', '')
);

ALTER TABLE households ALTER COLUMN invite_code SET NOT NULL;
CREATE UNIQUE INDEX households_invite_code_idx ON households (invite_code);

DROP TABLE invites;
