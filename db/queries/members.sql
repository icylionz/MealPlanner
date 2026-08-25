-- name: ListMembers :many
SELECT * FROM household_members ORDER BY created_at;

-- name: GetMember :one
SELECT * FROM household_members WHERE id = $1;

-- name: GetMemberByEmail :one
SELECT * FROM household_members WHERE lower(email) = lower($1);

-- name: GetPasswordlessMemberByName :one
SELECT * FROM household_members
WHERE lower(name) = lower($1) AND password_hash IS NULL
ORDER BY created_at
LIMIT 1;

-- name: CountMembers :one
SELECT count(*) FROM household_members;

-- name: CreateMember :one
INSERT INTO household_members (name, role, initials, color)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateMemberWithAuth :one
INSERT INTO household_members (name, role, initials, color, email, password_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: SetMemberCredentials :one
UPDATE household_members
SET email = $2, password_hash = $3
WHERE id = $1
RETURNING *;

-- name: DeleteMember :exec
DELETE FROM household_members WHERE id = $1 AND role <> 'owner';

-- name: CreateSession :exec
INSERT INTO sessions (token, member_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionMember :one
SELECT m.* FROM sessions s
JOIN household_members m ON m.id = s.member_id
WHERE s.token = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
