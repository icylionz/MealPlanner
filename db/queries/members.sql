-- name: ListMembers :many
SELECT * FROM household_members ORDER BY created_at;

-- name: GetMember :one
SELECT * FROM household_members WHERE id = $1;

-- name: CountMembers :one
SELECT count(*) FROM household_members;

-- name: CreateMember :one
INSERT INTO household_members (name, role, initials, color)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteMember :exec
DELETE FROM household_members WHERE id = $1 AND role <> 'owner';

-- name: CreateSession :exec
INSERT INTO sessions (token, member_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionMember :one
SELECT m.* FROM sessions s
JOIN household_members m ON m.id = s.member_id
WHERE s.token = $1 AND s.expires_at > now();

-- name: UpdateSessionMember :exec
UPDATE sessions SET member_id = $2 WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
