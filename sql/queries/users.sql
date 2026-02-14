-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name, avatar_url, settings_json)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, password_hash, display_name, avatar_url, settings_json, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, avatar_url, settings_json, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, avatar_url, settings_json, created_at, updated_at
FROM users
WHERE email = $1;

-- name: CheckUserEmailExists :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = $1) as exists;
