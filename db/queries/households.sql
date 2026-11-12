-- name: CreateHousehold :one
INSERT INTO households (name) VALUES ($1) RETURNING *;

-- name: GetHousehold :one
SELECT * FROM households WHERE id = $1 AND NOT is_template;

-- name: RenameHousehold :exec
UPDATE households SET name = $2 WHERE id = $1 AND NOT is_template;

-- name: GetTemplateHousehold :one
SELECT id FROM households WHERE is_template ORDER BY created_at LIMIT 1;

-- name: ListHouseholdsForAccount :many
SELECT h.id, h.name, m.role
FROM household_members m
JOIN households h ON h.id = m.household_id
WHERE m.account_id = $1 AND NOT h.is_template
ORDER BY m.created_at;

-- name: GetMember :one
SELECT * FROM household_members WHERE household_id = $1 AND account_id = $2;

-- name: ListMembers :many
SELECT m.id, m.account_id, m.role, m.initials, m.color, a.name, a.email
FROM household_members m
JOIN accounts a ON a.id = m.account_id
WHERE m.household_id = $1
ORDER BY m.created_at, m.id;

-- name: CountMembers :one
SELECT count(*) FROM household_members WHERE household_id = $1;

-- name: CreateMember :exec
INSERT INTO household_members (household_id, account_id, role, initials, color)
VALUES ($1, $2, $3, $4, $5);

-- name: DeleteMember :one
DELETE FROM household_members
WHERE id = $1 AND household_id = $2 AND role <> 'owner'
RETURNING account_id;

-- name: SetMemberRole :execrows
UPDATE household_members SET role = $3 WHERE household_id = $1 AND account_id = $2;

-- name: UpdateMemberInitials :exec
UPDATE household_members SET initials = $2 WHERE account_id = $1;

-- name: ClearSessionsHousehold :exec
UPDATE sessions SET active_household_id = NULL
WHERE account_id = $1 AND active_household_id = $2;

-- name: CreateInvite :one
INSERT INTO invites (household_id, code, expires_at, max_uses, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListInvites :many
SELECT * FROM invites
WHERE household_id = $1 AND revoked_at IS NULL AND expires_at > now()
  AND (max_uses IS NULL OR use_count < max_uses)
ORDER BY created_at DESC;

-- name: RevokeInvite :exec
UPDATE invites SET revoked_at = now() WHERE id = $1 AND household_id = $2;

-- name: UseInvite :one
UPDATE invites SET use_count = use_count + 1
WHERE lower(code) = lower($1) AND revoked_at IS NULL AND expires_at > now()
  AND (max_uses IS NULL OR use_count < max_uses)
RETURNING household_id;

-- name: CloneTemplateFoods :exec
INSERT INTO foods (id, household_id, name, description, prep_time_min, cook_time_min, servings,
                   yield_amount, default_unit, density_g_per_ml, density_source)
SELECT md5(f.id::text || sqlc.arg(household_id)::uuid::text)::uuid, sqlc.arg(household_id)::uuid, f.name, f.description,
       f.prep_time_min, f.cook_time_min, f.servings, f.yield_amount, f.default_unit,
       f.density_g_per_ml, f.density_source
FROM foods f
WHERE f.household_id = sqlc.arg(template_id)::uuid AND f.deleted_at IS NULL;

-- name: CloneTemplateAliases :exec
INSERT INTO food_aliases (food_id, alias)
SELECT md5(f.id::text || sqlc.arg(household_id)::uuid::text)::uuid, a.alias
FROM food_aliases a
JOIN foods f ON f.id = a.food_id
WHERE f.household_id = sqlc.arg(template_id)::uuid AND f.deleted_at IS NULL;

-- name: CloneTemplateTags :exec
INSERT INTO food_tags (food_id, tag)
SELECT md5(f.id::text || sqlc.arg(household_id)::uuid::text)::uuid, t.tag
FROM food_tags t
JOIN foods f ON f.id = t.food_id
WHERE f.household_id = sqlc.arg(template_id)::uuid AND f.deleted_at IS NULL;
