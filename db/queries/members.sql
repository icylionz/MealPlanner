-- Accounts: the login identity.

-- name: CreateAccount :one
INSERT INTO accounts (email, password_hash, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByEmail :one
SELECT * FROM accounts WHERE lower(email) = lower($1);

-- name: UpdateAccountProfile :one
UPDATE accounts SET name = $2, email = $3 WHERE id = $1
RETURNING *;

-- name: UpdateAccountPassword :exec
UPDATE accounts SET password_hash = $2 WHERE id = $1;

-- Households.

-- name: CreateHousehold :one
INSERT INTO households (name) VALUES ($1)
RETURNING *;

-- name: GetHousehold :one
SELECT * FROM households WHERE id = $1 AND is_template = false;

-- name: LockHousehold :one
SELECT id FROM households WHERE id = $1 AND is_template = false FOR UPDATE;

-- Invite lifecycle.

-- name: CreateInvite :one
INSERT INTO invites (household_id, code, expires_at, max_uses, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: InviteCodeExists :one
SELECT EXISTS (SELECT 1 FROM invites WHERE upper(code) = upper($1::text));

-- name: GetInviteByCodeForUpdate :one
SELECT * FROM invites WHERE upper(code) = upper($1::text) FOR UPDATE;

-- name: GetLatestInviteForHousehold :one
SELECT * FROM invites
WHERE household_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: RevokeActiveInvites :exec
UPDATE invites SET revoked_at = now()
WHERE household_id = $1 AND revoked_at IS NULL;

-- name: IncrementInviteUse :exec
UPDATE invites SET use_count = use_count + 1 WHERE id = $1;

-- Household membership (account <-> household join).

-- name: AddMember :one
INSERT INTO household_members (household_id, account_id, role, initials, color)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (household_id, account_id) DO NOTHING
RETURNING *;

-- name: CountHouseholdMembers :one
SELECT count(*) FROM household_members WHERE household_id = $1;

-- name: GetMembership :one
SELECT * FROM household_members WHERE household_id = $1 AND account_id = $2;

-- name: ListHouseholdMembers :many
SELECT m.id, m.household_id, m.account_id, m.role, m.initials, m.color, m.created_at,
       a.name AS account_name, a.email AS account_email
FROM household_members m
JOIN accounts a ON a.id = m.account_id
WHERE m.household_id = $1
ORDER BY m.created_at;

-- name: ListHouseholdsForAccount :many
SELECT h.id, h.name, h.created_at, m.role, m.initials, m.color
FROM household_members m
JOIN households h ON h.id = m.household_id
WHERE m.account_id = $1 AND h.is_template = false
ORDER BY h.created_at;

-- name: RemoveMember :exec
DELETE FROM household_members
WHERE household_id = $1 AND account_id = $2 AND role <> 'owner';

-- name: SetMemberRole :exec
UPDATE household_members SET role = $3
WHERE household_id = $1 AND account_id = $2;

-- Sessions: keyed on the account, carrying the active household.

-- name: CreateSession :exec
INSERT INTO sessions (token, account_id, active_household_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetSessionAccount :one
SELECT a.*, s.active_household_id, s.token
FROM sessions s
JOIN accounts a ON a.id = s.account_id
WHERE s.token = $1 AND s.expires_at > now();

-- name: SetActiveHousehold :exec
UPDATE sessions SET active_household_id = $2 WHERE token = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
