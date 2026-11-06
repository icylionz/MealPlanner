-- name: CreateHousehold :one
INSERT INTO households (name) VALUES ($1) RETURNING *;

-- name: GetHousehold :one
SELECT * FROM households WHERE id = $1;

-- name: RenameHousehold :exec
UPDATE households SET name = $2 WHERE id = $1;

-- name: TemplateHouseholdID :one
SELECT id FROM households WHERE is_template ORDER BY created_at LIMIT 1;

-- name: AddMember :one
INSERT INTO household_members (household_id, account_id, role, initials, color)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMembership :one
SELECT * FROM household_members WHERE household_id = $1 AND account_id = $2;

-- name: ListMembershipsForAccount :many
SELECT h.id, h.name, m.role
FROM household_members m
JOIN households h ON h.id = m.household_id
WHERE m.account_id = $1 AND NOT h.is_template
ORDER BY h.created_at;

-- name: ListMembers :many
SELECT m.id, m.account_id, m.role, m.initials, m.color, m.created_at, a.name, a.email
FROM household_members m
JOIN accounts a ON a.id = m.account_id
WHERE m.household_id = $1
ORDER BY m.role DESC, a.name;

-- name: SetMemberRole :execrows
UPDATE household_members SET role = $3 WHERE household_id = $1 AND account_id = $2;

-- name: RemoveMember :execrows
DELETE FROM household_members WHERE household_id = $1 AND account_id = $2;

-- name: CountOwners :one
SELECT count(*) FROM household_members WHERE household_id = $1 AND role = 'owner';

-- name: CreateInvite :one
INSERT INTO invites (household_id, code, expires_at, max_uses, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetInviteByCodeForUpdate :one
SELECT * FROM invites WHERE code = $1 FOR UPDATE;

-- name: IncrementInviteUse :exec
UPDATE invites SET use_count = use_count + 1 WHERE id = $1;

-- name: ListInvites :many
SELECT * FROM invites WHERE household_id = $1 ORDER BY created_at DESC;

-- name: RevokeInvite :execrows
UPDATE invites SET revoked_at = now()
WHERE id = $1 AND household_id = $2 AND revoked_at IS NULL;
