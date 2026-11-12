-- name: CreateAccount :one
INSERT INTO accounts (email, password_hash, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAccountByEmail :one
SELECT * FROM accounts WHERE lower(email) = lower($1);

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: UpdateAccountProfile :exec
UPDATE accounts SET name = $2, email = $3 WHERE id = $1;

-- name: UpdateAccountPassword :exec
UPDATE accounts SET password_hash = $2 WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token, account_id, active_household_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetSession :one
SELECT * FROM sessions WHERE token = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE account_id = $1 AND token <> $2;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- name: SetSessionHousehold :exec
UPDATE sessions SET active_household_id = $2 WHERE token = $1;
