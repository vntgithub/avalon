---
name: avalon-go-backend
overview: Design a Go + PostgreSQL backend for an Avalon game with anonymous room-based play, persistent game state, and WebSocket-based realtime interactions.
todos:
  - id: bootstrap-go-project
    content: Bootstrap Go module, main server in cmd/server, and basic HTTP router setup.
    status: completed
  - id: design-db-schema
    content: Design and write SQL migrations for rooms, players, games, snapshots, moves, and chat tables in PostgreSQL.
    status: in_progress
  - id: implement-room-endpoints
    content: Implement REST endpoints for creating rooms, joining rooms, and fetching room/latest game details.
    status: pending
  - id: wire-postgres-store
    content: Create store layer using pgx or similar to handle queries and transactions.
    status: pending
  - id: implement-websocket-hub
    content: Implement per-room WebSocket hub with authentication and basic chat broadcasting.
    status: pending
  - id: build-game-engine
    content: Implement configurable Avalon-style game engine handling phases, roles, and actions with persistence via events and snapshots.
    status: pending
  - id: add-tests
    content: Write unit tests for game engine and integration tests for room and WebSocket flows.
    status: pending
isProject: false
---

## High-level architecture

- **Tech stack**: Go (net/http or lightweight router like chi), PostgreSQL (via `pgx` or similar), WebSockets for realtime (e.g. `gorilla/websocket` or stdlib `websocket` once available), JSON over HTTP/WebSocket.
- **Process model**: Single HTTP server exposing REST endpoints for room/game management plus a WebSocket endpoint per room; in-memory room hubs coordinated with persistent state in PostgreSQL.
- **Player identity model**: Anonymous users identified per-room by a display name and generated `player_id`; no global accounts.
- **Game rules model**: A configurable game engine where roles, phases, and allowed actions are data-driven so future social-deduction games can reuse the backend.

## Project structure

- `**cmd/server/main.go**`: Entry point, configuration loading (env vars), DB connection pool, HTTP router setup, WebSocket upgrader wiring, graceful shutdown.
- `**internal/http**`: HTTP handlers, request/response DTOs, middleware (logging, CORS, request ID, panic recovery).
- `**internal/rooms**`: Room domain logic (create room, join room, load latest game, password check, room lifecycle).
- `**internal/games**`: Game domain logic (game creation, phases, turns, voting, outcomes, serialization of game state snapshots).
- `**internal/realtime**`: WebSocket hub management (per-room hub, broadcasting, connection management, message routing to domain services).
- `**internal/store**`: PostgreSQL access layer (SQL queries, migrations path, transactions), using a lightweight approach (e.g. `pgx` + hand-written SQL or `sqlc`).
- `**migrations/**`: SQL migrations for PostgreSQL schema (managed via `golang-migrate` or similar tool).

## Database schema design (PostgreSQL)

- `**rooms**`
  - `id` (UUID or short code), `code` (user-facing room code), `password_hash` (nullable), `created_at`, `updated_at`.
  - `settings_json` (JSONB) storing room-level settings like number of players, meeting time per round, variant flags.
- `**room_players**`
  - `id` (UUID), `room_id` (FK), `display_name`, `created_at`.
  - Optional `is_host` flag to allow special actions.
- `**games**`
  - `id` (UUID), `room_id` (FK), `status` (e.g. `waiting`, `in_progress`, `finished`), `created_at`, `ended_at`.
  - `config_json` (JSONB) for rules configuration (roles, phases, victory conditions).
- `**game_players**`
  - `id` (UUID), `game_id` (FK), `room_player_id` (FK), role assignment, join/leave timestamps.
- `**game_state_snapshots**`
  - `id` (UUID), `game_id` (FK), `version` (int), `state_json` (JSONB) holding full engine state at key points (start of phase, after vote, etc.).
  - Allows loading the latest snapshot when a user joins.
- `**moves` / `events**`
  - `id` (UUID), `game_id` (FK), `room_player_id` (FK), `type` (e.g. `vote`, `chat`, `phase_change`, `role_reveal`), `payload_json` (JSONB), `created_at`.
  - Append-only log that the engine can consume/replay.
- `**chat_messages**` (optional separate table if needed)
  - `id`, `room_id`/`game_id`, `room_player_id`, `message`, `created_at`.

## Core use cases & flows

### Create room (optional password)

- **HTTP endpoint**: `POST /api/rooms`
  - Request: room settings (number of players, time per round, rule preset), optional `password`, creator display name.
  - Server:
    - Validate payload, hash password if provided.
    - Generate `room.code` (short, joinable by humans) and `room.id` (UUID).
    - Insert `room` + initial `room_player` (creator, host) and an initial `game` entity with configured rules.
  - Response: room `code`, room `id`, host `room_player_id` and a short-lived auth token for WebSocket (e.g. signed JWT or HMAC with room+player IDs).

### Join room by id/code

- **HTTP endpoint**: `POST /api/rooms/{code}/join`
  - Request: display name, optional password.
  - Server:
    - Look up room by `code`, verify password if room has `password_hash`.
    - Create `room_player` row, associate with latest game (creating `game_player`).
    - Return `room` summary, latest game summary, `room_player_id`, `game_player_id`, and WebSocket auth token.

### Load latest game on join

- **HTTP endpoint**: `GET /api/rooms/{code}`
  - Returns: room info, latest game descriptor (`id`, status, basic config), and latest `game_state_snapshot` for that game.
  - Logic:
    - Find most recent `game` for `room_id` by `created_at` or `status`.
    - Fetch highest-version snapshot and serialize to client.

### Start new game in a room

- **HTTP endpoint**: `POST /api/rooms/{code}/games`
  - Host can create a fresh `game` in a room with a given rules preset; previous games remain in history.

## WebSocket realtime design

### Endpoint & authentication

- **Endpoint**: `GET /ws/rooms/{code}` (or `/ws/rooms/{code}/connect`).
- **Client connect**: passes `player_id` and auth token (from join/create) via query params or headers.
- **Server**:
  - Validates token, loads room + player, attaches connection to per-room hub.
  - Subscribes player to relevant streams: game events, chat, room lobby updates.

### Message envelope format

- **Common JSON structure**:
  - Client → server: `{ "type": "vote" | "chat" | "action" | "system", "correlation_id": string, "payload": {...} }`.
  - Server → client: `{ "type": "event" | "state" | "error", "event": "vote_recorded" | "phase_changed" | ... , "payload": {...} }`.
- **Types to support initially**:
  - `chat`: send chat message to room/game.
  - `vote`: submit a vote for the current phase.
  - `action`: generalized game action (e.g., propose team, approve/reject, use power) encoded generically so new rules can be added via config.
  - `sync_state`: client requests full state; server replies with `state` message using latest snapshot.

### Hub & routing

- **Per-room hub**:
  - Maintains map of `player_id -> connection`.
  - Receives messages from connections, validates, forwards to domain services (`games`/`rooms`).
  - Broadcasts resulting events (e.g., new chat message, updated state, phase change) to all or subset of players.
- **Backpressure & lifecycle**:
  - Use read and write pumps with channels and deadlines.
  - Handle `ping/pong`, disconnect idle clients, and cleanup on error.

## Game engine & rules configuration

### Rules configuration model

- **Preset definitions** (e.g. classic Avalon):
  - Stored as JSON or Go structs (and mirrored in DB `config_json`): roles list, phase sequence, allowed actions per phase, visibility rules.
- **Customizable games**:
  - Treat roles and phases as generic data: each phase has
    - `name` (e.g. `team_selection`, `mission_vote`, `mission_resolution`).
    - Allowed actions (`propose_team`, `approve_team`, `submit_mission_vote`, etc.).
    - Constraints (e.g., number of players on team, timeout durations).

### Engine responsibilities

- **State machine**:
  - Maintain a state struct (current phase, round number, leader, teams, past mission results, player roles, etc.).
  - Apply incoming events/moves to produce new state and derived events.
- **Validation**:
  - Ensure that only legal actions for the current phase are applied (e.g., only leader can propose team, only eligible players can vote).
- **Persistence integration**:
  - On each significant state change, append an `event` and create/update `game_state_snapshot` (e.g., at least once per phase).

## Room & game lifecycle logic

- **Room lifecycle**:
  - A room can host many games; its `settings_json` acts as defaults for future games.
  - Deleting rooms is optional (soft delete); older rooms can be archived.
- **Game lifecycle**:
  - `waiting` → `in_progress` → `finished`.
  - When a game finishes, host can trigger creating a new game via REST endpoint; `GET /api/rooms/{code}` always points to latest game.
- **Rejoin / reconnect**:
  - On WebSocket reconnect, server uses `room_player_id` and token to restore session and push latest `state` message.

## Security & validation

- **Room password**:
  - Store only hashes (e.g., bcrypt/argon2) in `rooms.password_hash`.
  - On join, compare provided password with hash.
- **Anonymous identity**:
  - Enforce unique display names per room where possible.
  - Use opaque `player_id` and signed tokens to prevent impersonation.
- **Input validation**:
  - Validate all HTTP requests and WebSocket payloads (JSON binding with explicit structs and validation tags).
- **Rate limiting & abuse prevention** (later phase):
  - IP-based basic throttling for joins and chat.

## Testing strategy

- **Unit tests**:
  - Focus heavily on `games` package (state machine, rule validation, event generation) using table-driven tests.
- **Integration tests**:
  - Spin up ephemeral Postgres (e.g., Docker + testcontainer or dedicated test DB) and exercise room/game flows via HTTP handlers.
- **WebSocket tests**:
  - Use Go WebSocket client in tests to simulate multiple players joining, voting, and chatting; assert broadcasted events and state transitions.

## Suggested initial implementation order

1. **Bootstrap project & dependencies**: basic Go module, HTTP router, config, logging.
2. **Set up PostgreSQL & migrations**: create schema tables in `migrations/` and wire `internal/store` using `pgx`.
3. **Implement room REST endpoints**: `POST /api/rooms`, `POST /api/rooms/{code}/join`, `GET /api/rooms/{code}`.
4. **Implement minimal game model**: `games` package with creation, association with rooms, and snapshotting.
5. **Add WebSocket hub**: `/ws/rooms/{code}` with connect/auth, broadcast, and simple `chat` message type.
6. **Extend game engine**: add phases, roles, and voting actions driven by config, persisted via `moves` and `game_state_snapshots`.
7. **Polish & harden**: add validation, basic rate limiting hooks, and comprehensive tests for main flows.
