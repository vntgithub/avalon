# Phase 3: Implement room REST endpoints

**Goal**: Expose REST API for creating rooms, joining rooms by code, and loading the latest game for a room. Return WebSocket auth tokens for use in Phase 5.

See **avalon-go-backend-master.md** for core use cases (Create room, Join room, Load latest game).

---

## Summary

- **POST /api/rooms** — create room (optional password), creator display name, room settings; return room code, room id, host `room_player_id`, WebSocket auth token.
- **POST /api/rooms/{code}/join** — join by room code; body: display name, optional password; return room summary, latest game summary, `room_player_id`, `game_player_id`, WebSocket auth token.
- **GET /api/rooms/{code}** — get room info, latest game descriptor, and latest `game_state_snapshot` for that game (for (re)join and sync).

All requests/responses use JSON. Validate input; hash and verify room passwords (bcrypt/argon2).

---

## Concrete steps

1. **Request/response DTOs**
   - Create room: request body with display name, optional password, optional settings (e.g. max players, rule preset). Response: `room_id`, `code`, `room_player_id`, `token` (and optionally `expires_at`).
   - Join room: request body with display name, optional password. Response: room summary, latest game summary, `room_player_id`, `game_player_id`, `token`.
   - GET room: response includes room metadata, latest game (`id`, status, config summary), and latest snapshot (`state_json` or equivalent).

2. **Validation**
   - Display name: non-empty, length limit, optional uniqueness per room.
   - Password: length limits; hash with bcrypt or argon2 before storing.
   - Room code in path: validate format (e.g. alphanumeric short code).

3. **Create room flow**
   - Validate payload → hash password → generate UUID for room and short human-readable `code` (ensure uniqueness).
   - In a transaction: insert `rooms`, insert `room_players` (creator, `is_host` true), create initial `games` row (e.g. status `waiting`) and optionally `game_players` for host.
   - Generate signed WebSocket auth token (JWT or HMAC) containing room id and room_player id (and optional expiry).
   - Return 201 with JSON response.

4. **Join room flow**
   - Resolve room by `code` (path param). If not found → 404.
   - If room has `password_hash`, verify provided password → 401 if invalid.
   - In a transaction: insert `room_players`, get latest game for room, insert `game_players` for that game.
   - Generate WebSocket auth token.
   - Return 200 with room summary, latest game summary, ids, and token.

5. **Get room flow**
   - Resolve room by `code`. If not found → 404.
   - Load latest game for room (by `created_at` or status). Load latest snapshot for that game (max `version`).
   - Return 200 with room info, game descriptor, and snapshot state (or 204/empty snapshot if none).

6. **Errors**
   - 400 for validation errors (with clear message).
   - 401 for wrong password.
   - 404 for unknown room code.
   - 409 if join would violate constraints (e.g. duplicate display name if enforced).
   - 500 for internal errors (log with request ID).

---

## References

- `internal/httpapi/room_handler.go` — create, join, get room handlers.
- `internal/httpapi/router.go` — register `POST /api/rooms`, `POST /api/rooms/{code}/join`, `GET /api/rooms/{code}`.
- `internal/store/room.go` — create room, get by code, add player, get latest game/snapshot.

---

## Acceptance criteria

- [ ] POST /api/rooms with display name creates room and returns code, ids, and token.
- [ ] POST /api/rooms/{code}/join with valid (optional) password adds player and returns token.
- [ ] GET /api/rooms/{code} returns room, latest game, and latest snapshot.
- [ ] Invalid password on join returns 401; unknown code returns 404.
- [ ] All request bodies validated; invalid input returns 400 with clear message.
