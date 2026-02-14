# Phase 7: Polish & harden

**Goal**: Harden security and validation, add rate-limiting hooks, and implement comprehensive tests (unit tests for game engine, integration tests for room/game HTTP and WebSocket).

See **avalon-go-backend-master.md** for Security & validation and Testing strategy.

---

## Summary

- **Security**: Confirm password hashing (bcrypt/argon2), opaque player ids, signed tokens; validate all HTTP and WebSocket inputs with explicit structs and validation.
- **Rate limiting**: Add hooks for IP-based throttling on join and chat (e.g. middleware or per-handler checks); can be in-memory or external later.
- **Tests**: Unit tests for game engine (state machine, rule validation, event generation); integration tests for room/game REST and WebSocket flows; optional load/smoke tests.

---

## Concrete steps

1. **Security review**
   - **Passwords**: Ensure `rooms.password_hash` is never logged; only bcrypt/argon2 hashes stored; comparison constant-time.
   - **Tokens**: WebSocket and any API tokens are signed (HMAC/JWT); include room + player identity and expiry; reject tampered or expired tokens.
   - **Input validation**: All HTTP request bodies and WebSocket payloads bound to Go structs with validation (e.g. `validate` tags or manual checks). Reject unknown fields if desired; enforce max lengths (display name, message, payload size).
   - **Anonymous identity**: No global accounts; enforce unique display names per room if specified; use opaque `player_id` (UUID) in responses and tokens.

2. **Rate limiting**
   - Define interface or middleware for rate limit check (e.g. by IP or by room+player).
   - Apply to: POST /api/rooms (create), POST /api/rooms/{code}/join, and WebSocket chat message handler. Return 429 when over limit with optional Retry-After.
   - Implementation can be in-memory (e.g. sliding window per IP) or placeholder that always allows; document extension point for Redis etc.

3. **Unit tests — game engine**
   - Table-driven tests: given initial state and a move, expect new state and/or error.
   - Cover: valid transitions (team proposal, approval, mission vote, resolution), invalid actor, invalid phase, invalid payload, victory conditions.
   - Mock or use real store for event/snapshot persistence in tests; prefer deterministic replay.

4. **Integration tests — HTTP**
   - Use ephemeral Postgres (e.g. Docker, testcontainers, or CI DB). Start app (or only store layer) and run:
     - Create room → GET room → join room → GET room again; assert response shapes and latest game/snapshot.
     - Create room with password → join with wrong password (expect 401); join with correct password (expect 200).
     - POST /api/rooms/{code}/games as host → new game created; GET room returns new game as latest.
   - Clean up DB state between tests (truncate or migrate down/up).

5. **Integration tests — WebSocket**
   - Use Go WebSocket client (e.g. gorilla/websocket) to connect with valid token; send chat; assert broadcast received by other client in same room.
   - Connect second client; send vote/action; assert engine state and broadcast (e.g. event type and payload). Optionally test invalid token and expect disconnect.
   - Test reconnect: disconnect and reconnect with same token; send sync_state; assert latest state matches.

6. **Room & game lifecycle edge cases**
   - Rejoin: same room_player_id can reconnect; GET /api/rooms/{code} and sync_state return consistent latest game and snapshot.
   - Multiple games: after finishing a game, host starts new game; GET room returns new game; old game still in history (if you have a history endpoint, assert it).

7. **Logging and observability**
   - Ensure request ID and key identifiers (room_id, game_id, player_id) in logs for debugging; no secrets in logs.
   - Optional: metrics (request count, WS connection count, game count) for later.

---

## References

- `internal/httpapi/middleware.go` — rate limit middleware.
- `internal/websocket/ws_handler.go` — validation, token verification.
- `internal/games/` — engine unit tests (e.g. `engine_test.go`).
- `internal/httpapi/` — handler tests or integration tests (e.g. `room_test.go`, `game_test.go`).
- `internal/websocket/integration_test.go` — WebSocket integration tests.

---

## Acceptance criteria

- [x] All HTTP and WebSocket inputs validated; invalid input returns 400/error and does not crash.
- [x] Rate limiting applied to create room, join room, and chat; 429 returned when configured limit exceeded.
- [x] Game engine unit tests cover valid/invalid moves and phase transitions; all pass.
- [x] Integration tests: create/join/get room and start game; WebSocket chat and (if implemented) vote/action; reconnect and sync_state. All pass.
- [x] No secrets or raw passwords in logs; tokens are signed and expiry enforced.
