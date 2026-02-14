-- name: CreateChatMessage :one
INSERT INTO chat_messages (room_id, game_id, room_player_id, message)
VALUES ($1, $2, $3, $4)
RETURNING id, room_id, game_id, room_player_id, message, created_at;
