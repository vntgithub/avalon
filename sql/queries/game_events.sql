-- name: CreateGameEvent :one
INSERT INTO game_events (game_id, room_player_id, type, payload_json)
VALUES ($1, $2, $3, $4)
RETURNING id, game_id, room_player_id, type, payload_json, created_at;

-- name: GetGameEventsByGameId :many
SELECT id, game_id, room_player_id, type, payload_json, created_at
FROM game_events
WHERE game_id = $1
ORDER BY created_at ASC;

-- name: GetGameEventsByGameIdAfter :many
SELECT id, game_id, room_player_id, type, payload_json, created_at
FROM game_events
WHERE game_id = $1 AND created_at > $2
ORDER BY created_at ASC;

-- name: GetRoomPlayersByGameId :many
SELECT rp.id, rp.room_id, rp.display_name, rp.is_host, rp.created_at
FROM room_players rp
INNER JOIN game_players gp ON gp.room_player_id = rp.id
WHERE gp.game_id = $1
ORDER BY rp.created_at ASC;
