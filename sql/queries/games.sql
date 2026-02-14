-- name: GetRoomById :one
SELECT id, code, password_hash, settings_json, created_at, updated_at
FROM rooms
WHERE id = $1;

-- name: GetRoomPlayersByRoomId :many
SELECT id, room_id, display_name, is_host, created_at
FROM room_players
WHERE room_id = $1
ORDER BY created_at ASC;

-- name: CreateGame :one
INSERT INTO games (room_id, status, config_json)
VALUES ($1, $2, $3)
RETURNING id, room_id, status, config_json, created_at, ended_at;

-- name: CreateGamePlayer :one
INSERT INTO game_players (game_id, room_player_id, role)
VALUES ($1, $2, $3)
RETURNING id, game_id, room_player_id, role, joined_at, left_at;

-- name: GetGameById :one
SELECT id, room_id, status, config_json, created_at, ended_at
FROM games
WHERE id = $1;

-- name: GetGamesByRoomId :many
SELECT id, room_id, status, config_json, created_at, ended_at
FROM games
WHERE room_id = $1
ORDER BY created_at DESC;

-- name: CreateGameStateSnapshot :one
INSERT INTO game_state_snapshots (game_id, version, state_json)
VALUES ($1, $2, $3)
RETURNING id, game_id, version, state_json, created_at;

-- name: GetLatestGameStateSnapshotByGameId :one
SELECT id, game_id, version, state_json, created_at
FROM game_state_snapshots
WHERE game_id = $1
ORDER BY version DESC
LIMIT 1;

-- name: UpdateGameStatus :exec
UPDATE games
SET status = $2, ended_at = $3
WHERE id = $1;
