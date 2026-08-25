-- Member login: each household member may hold an email + bcrypt password hash.
-- Nullable so seeded (passwordless) members survive and can be claimed later by
-- registering the same email. Email is unique case-insensitively when present.
ALTER TABLE household_members
    ADD COLUMN email         text,
    ADD COLUMN password_hash text;

CREATE UNIQUE INDEX household_members_email_idx
    ON household_members (lower(email))
    WHERE email IS NOT NULL;
