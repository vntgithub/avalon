# Phase 5: Add WebSocket hub

**Goal**: Implement per-room WebSocket endpoint with token authentication, a hub that maps `player_id` to connection, and basic `chat` message type with broadcast. Prepare for game events in Phase 6.

See **avalon-go-backend-master.md** for WebSocket realtime design (endpoint, auth, message envelope, hub).

---

## Summary

- **Endpoint**: `GET /ws/rooms/{code}` (or `/ws/rooms/{code}/connect`). Client sends `player_id` and auth token (from create/join) via query params or headers.
- **Auth**: Validate token, load room + player, attach connection to per-room hub.
- **Hub**: In-memory map of room code → hub; each hub holds `player_id` → connection. Broadcast messages to all (or a subset) in the room.
- **Messages**: Common JSON envelope. Initially support `chat`: client sends chat, server persists (optional) and broadcasts to room.
- **Lifecycle**: Read/write pumps, ping/pong, disconnect and cleanup on error or close.

---

## Concrete steps

1. **Route and upgrade**
   - Register `GET /ws/rooms/{code}`. Extract `code` from path; optional query params: `player_id`, `token` (or pass token in header).
   - Upgrade HTTP to WebSocket (e.g. `gorilla/websocket`). On upgrade failure, return 4xx with body or close with reason.

2. **Authentication**
   - Before or immediately after upgrade: validate token (verify signature, parse room_id + room_player_id). Load room by code and ensure it matches token; load room_player by id and ensure they belong to room.
   - If invalid: close connection with appropriate code (e.g. 4401 Unauthorized) or respond with an error message over WS then close.
   - Attach `room_id`, `room_player_id`, `display_name` to the connection context or wrapper.

3. **Per-room hub**
   - Global (or injected) registry: `map[roomCode]Hub` or `sync.Map`. Hub struct: `map[playerID]Connection`, mutex, `broadcast(channel)`, `register(conn)`, `unregister(conn)`.
   - On successful auth: get or create hub for room code, register connection. On disconnect: unregister, and if hub empty, remove from registry (optional).

4. **Message envelope**
   - Client → server: `{ "type": "chat" | "vote" | "action" | "system", "correlation_id": "...", "payload": { ... } }`.
   - Server → client: `{ "type": "event" | "state" | "error", "event": "chat" | "phase_changed" | ..., "payload": { ... } }`.
   - For Phase 5, handle only `type: "chat"`. Payload e.g. `{ "message": "..." }`. Validate (max length, sanitize if needed).

5. **Chat flow**
   - On `chat` message: optionally persist to `chat_messages` table (room_id, room_player_id, message). Broadcast to hub: send same message as `type: "event", event: "chat"` with payload including sender display_name and message.
   - All connected clients in the room receive the broadcast.

6. **Read/write pumps**
   - Goroutine 1: read loop — decode JSON, dispatch by `type` to handler (chat, etc.). On error or close, signal disconnect.
   - Goroutine 2: write loop — read from a per-connection send channel; write JSON frames. Use write deadline for ping/backpressure.
   - Ping/pong: use WebSocket ping/pong or application-level ping; close connection after missed pong timeout.
   - On disconnect: unregister from hub, close send channel, exit pumps.

7. **Backpressure**
   - Buffered send channel per connection (e.g. cap 256). If full, drop or close connection to avoid slow clients blocking hub.

---

## References

- `internal/websocket/` or `internal/realtime/` — hub, connection wrapper, message types.
- `internal/httpapi/router.go` — register WebSocket route; pass hub registry and token validator.
- `internal/websocket/ws_handler.go` — upgrade, auth, register, read/write pumps, chat handler.

---

## Acceptance criteria

- [ ] Connecting with valid token and room code joins the room hub; invalid token is rejected.
- [ ] Sending a `chat` message broadcasts to all other clients in the same room with correct envelope.
- [ ] Disconnecting unregisters the client; hub does not leak connections.
- [ ] Ping/pong or write deadline in place; idle clients are disconnected after timeout.
