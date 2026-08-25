DROP INDEX IF EXISTS household_members_email_idx;
ALTER TABLE household_members
    DROP COLUMN email,
    DROP COLUMN password_hash;
