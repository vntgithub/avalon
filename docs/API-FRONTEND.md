# Avalon API — Frontend Reference

Use this document as context when building the frontend. All endpoints return JSON unless noted. Error responses are plain text unless specified.

**Base path:** `/` (e.g. `https://your-api.example.com/`)

**CORS:** Enabled. Allowed methods: `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`. Allowed headers: `Accept`, `Authorization`, `Content-Type`, `X-Requested-With`.

---

## Authentication

Protected endpoints require a **Bearer token** in the `Authorization` header:

```http
Authorization: Bearer <token>
```

You get the token from:

- **Register:** `POST /api/auth/register` → response includes `token` and `user`
- **Login:** `POST /api/auth/login` → response includes `token` and `user`
- **Create room:** `POST /api/rooms` → response includes `token` (room-scoped, for WebSocket)
- **Join room:** `POST /api/rooms/{code}/join` → response includes `token` (room-scoped, for WebSocket)

Use the **session token** (from register/login) for REST calls like `GET /api/users/me`, `POST /api/rooms`, `POST /api/rooms/{code}/join`, `POST /api/rooms/{code}/games`. Use the **room token** (from create/join room) for the room WebSocket `GET /ws/rooms/{code}`.

---

## Health

| Method | Path       | Auth | Description                |
|--------|------------|------|----------------------------|
| GET    | `/healthz` | No   | Liveness/readiness check.  |

**Response 200**

```json
{ "status": "ok" }
```

---

## Auth

### Register

**POST** `/api/auth/register`

Create a new account. Returns user and session token.

**Auth:** None (rate-limited by IP).

**Request body**

```json
{
  "email": "string",         // required, valid email, max 256 chars
  "password": "string",      // required, 8–128 chars
  "display_name": "string"   // required, 1–64 chars (trimmed)
}
```

**Responses**

- **201** — Created. Body: `AuthResponse` (see below).
- **400** — Validation error (plain text message).
- **409** — Email already registered (plain text).
- **500** — Server error (plain text).

### Login

**POST** `/api/auth/login`

Authenticate with email and password. Returns user and session token.

**Auth:** None (rate-limited by IP).

**Request body**

```json
{
  "email": "string",    // required
  "password": "string"  // required
}
```

**Responses**

- **200** — OK. Body: `AuthResponse`.
- **400** — Bad request (plain text).
- **401** — Invalid email or password (plain text).
- **500** — Server error (plain text).

**AuthResponse**

```json
{
  "user": { /* User */ },
  "token": "string",
  "expires_at": "string"   // ISO8601 or empty
}
```

---

## Users

### Get current user

**GET** `/api/users/me`

Return the authenticated user’s profile.

**Auth:** Required (Bearer session token).

**Responses**

- **200** — OK. Body: `User`.
- **401** — Unauthorized (plain text).

**User**

```json
{
  "id": "string",
  "email": "string",
  "display_name": "string",
  "avatar_url": "string",
  "created_at": "string",
  "updated_at": "string"
}
```

---

## Rooms

### Create room

**POST** `/api/rooms`

Create a new room. Caller becomes host. Display name is taken from the authenticated user profile.

**Auth:** Required (Bearer session token). Rate-limited by IP.

**Request body**

```json
{
  "password": "string",   // optional, max 128 chars
  "settings": {}          // optional, arbitrary object
}
```

**Responses**

- **201** — Created. Body: `CreateRoomResponse`.
- **400** — Bad request (e.g. password length, invalid body) (plain text).
- **401** — Unauthorized (plain text).
- **500** — Server error (plain text).

**CreateRoomResponse**

```json
{
  "room": { /* Room */ },
  "room_player": { /* RoomPlayer */ },
  "token": "string",       // room-scoped token for WebSocket
  "expires_at": "string"   // optional, ISO8601
}
```

### Get room

**GET** `/api/rooms/{code}`

Get room details and latest game state. No authentication required.

**Path**

- `code` — Room code (6 alphanumeric characters).

**Responses**

- **200** — OK. Body: `GetRoomResponse`.
- **400** — Invalid room code (plain text).
- **404** — Room not found (plain text).
- **500** — Server error (plain text).

**GetRoomResponse**

```json
{
  "room": { /* Room */ },
  "latest_game": { /* Game */ },           // optional, if room has games
  "latest_game_state_snapshot": {}        // optional, game state object
}
```

**Room**

```json
{
  "id": "string",
  "code": "string",
  "settings": {},
  "created_at": "string",
  "updated_at": "string"
}
```

**RoomPlayer**

```json
{
  "id": "string",
  "room_id": "string",
  "display_name": "string",
  "is_host": true,
  "user_id": "string",
  "created_at": "string"
}
```

### Join room

**POST** `/api/rooms/{code}/join`

Join an existing room. Display name is taken from the authenticated user profile.

**Auth:** Required (Bearer session token). Rate-limited by IP.

**Path**

- `code` — Room code (6 alphanumeric). Can also be sent in body for convenience; path takes precedence.

**Request body**

```json
{
  "code": "string",      // optional if in path
  "password": "string"   // optional, required if room has password
}
```

**Responses**

- **200** — OK. Body: `JoinRoomResponse`.
- **400** — Bad request (plain text).
- **401** — Unauthorized or password required/invalid (plain text).
- **404** — Room not found (plain text).
- **409** — Display name already taken in this room (plain text).
- **500** — Server error (plain text).

**JoinRoomResponse**

```json
{
  "room": { /* Room */ },
  "room_player": { /* RoomPlayer */ },
  "latest_game": { /* Game */ },
  "game_player": { /* GamePlayer */ },    // optional, if there is a current game
  "latest_game_state_snapshot": {},
  "token": "string",
  "expires_at": "string"
}
```

---

## Games

### Create game

**POST** `/api/rooms/{code}/games`

Start a new game in the room. Only the room host may call this. Room player is resolved from the authenticated user.

**Auth:** Required (Bearer session token).

**Path**

- `code` — Room code (6 alphanumeric).

**Request body**

```json
{
  "config": {}   // optional, game config object
}
```

**Responses**

- **201** — Created. Body: `CreateGameResponse`.
- **400** — Bad request or room has no players (plain text).
- **401** — Unauthorized (plain text).
- **403** — Only host can start a new game, or user not in room (plain text).
- **404** — Room not found (plain text).
- **500** — Server error (plain text).

**CreateGameResponse**

```json
{
  "game": { /* Game */ },
  "players": [ { /* GamePlayer */ } ],
  "latest_game_state_snapshot": {}
}
```

**Game**

```json
{
  "id": "string",
  "room_id": "string",
  "status": "string",   // "waiting" | "in_progress" | "finished"
  "config": {},
  "created_at": "string",
  "ended_at": "string"
}
```

**GamePlayer**

```json
{
  "id": "string",
  "game_id": "string",
  "room_player_id": "string",
  "role": "string",
  "joined_at": "string",
  "left_at": "string"
}
```

---

## WebSockets

### Room WebSocket

**GET** `/ws/rooms/{code}`

Real-time room channel (chat, presence, etc.). Requires room-scoped token from create or join room.

**Auth:** Room token via query or header:

- Query: `?token=<room_token>`
- Header: `Authorization: Bearer <room_token>`

**Path**

- `code` — Room code (6 alphanumeric).

**Responses**

- **101** — Switching Protocols (WebSocket upgrade).
- **400** — Missing room code (plain text).
- **401** — Missing/invalid token or room does not match token (plain text).
- **404** — Room not found (plain text).

After upgrade, use the WebSocket for bidirectional messages (format is implementation-specific; see backend event types if needed).

### Game WebSocket

**GET** `/api/rooms/{code}/games/{game_id}/ws`

Game events stream for a specific game. Optional query: `room_player_id=<id>` to identify the client.

**Auth:** None on upgrade (game is public by room; identity via `room_player_id` if needed).

**Path**

- `code` — Room code (6 alphanumeric).
- `game_id` — Game UUID.

**Responses**

- **101** — Switching Protocols (WebSocket upgrade).
- **400** — Missing `code` or `game_id` (plain text).
- **404** — Room not found (plain text).

After upgrade, client receives game state updates and can send actions according to the game protocol.

---

## Error handling

- **4xx/5xx** — Many endpoints return a **plain text** body with a short message (e.g. `"email is required"`, `"room not found"`).
- **429** — Rate limit exceeded (e.g. create/join/chat). Body: plain text.
- Always send `Content-Type: application/json` for JSON request bodies and expect `Content-Type: application/json` for successful JSON responses.

---

## Validation summary (frontend)

| Field          | Rules                                      |
|----------------|--------------------------------------------|
| email          | Required, valid format, max 256 chars      |
| password       | Required for auth, 8–128 chars              |
| display_name   | Required, 1–64 chars (trimmed)             |
| room code      | Exactly 6 alphanumeric chars                |
| room password  | Max 128 chars                              |

---

## Quick endpoint list

| Method | Path                           | Auth        | Purpose           |
|--------|--------------------------------|------------|-------------------|
| GET    | `/healthz`                     | No         | Health check      |
| POST   | `/api/auth/register`          | No         | Register          |
| POST   | `/api/auth/login`             | No         | Login             |
| GET    | `/api/users/me`               | Bearer     | Current user      |
| POST   | `/api/rooms`                  | Bearer     | Create room       |
| GET    | `/api/rooms/{code}`           | No         | Get room          |
| POST   | `/api/rooms/{code}/join`      | Bearer     | Join room         |
| POST   | `/api/rooms/{code}/games`     | Bearer     | Start game        |
| GET    | `/ws/rooms/{code}`            | Room token | Room WebSocket    |
| GET    | `/api/rooms/{code}/games/{id}/ws` | No     | Game WebSocket    |

Swagger UI is available at **GET /docs/** when the server is running (interactive try-it-out and full schema).
