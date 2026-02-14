-- name: CheckRoomCodeExists :one
SELECT EXISTS(SELECT 1 FROM rooms WHERE code = $1) as exists;

-- name: GetRoomByCode :one
SELECT id, password_hash, settings_json, created_at, updated_at
FROM rooms
WHERE code = $1;

-- name: CreateRoom :one
INSERT INTO rooms (code, password_hash, settings_json)
VALUES ($1, $2, $3)
RETURNING id, code, created_at, updated_at;

-- name: CheckDisplayNameExists :one
SELECT EXISTS(SELECT 1 FROM room_players WHERE room_id = $1 AND display_name = $2) as exists;

-- name: CreateRoomPlayer :one
INSERT INTO room_players (room_id, display_name, is_host, user_id)
VALUES ($1, $2, $3, $4)
RETURNING id, room_id, display_name, is_host, user_id, created_at;

-- name: GetRoomCodeById :one
SELECT code FROM rooms WHERE id = $1;

-- name: GetRoomPasswordHashById :one
SELECT password_hash FROM rooms WHERE id = $1;

-- name: CountRoomsById :one
SELECT COUNT(*) FROM rooms WHERE id = $1;

-- name: CountRoomPlayersById :one
SELECT COUNT(*) FROM room_players WHERE id = $1;

-- name: CountRoomPlayersByRoomId :one
SELECT COUNT(*) FROM room_players WHERE room_id = $1;

-- name: GetRoomPlayerByRoomIdAndUserId :one
SELECT id, room_id, display_name, is_host, user_id, created_at
FROM room_players
WHERE room_id = $1 AND user_id = $2;
