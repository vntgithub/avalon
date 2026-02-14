# Avalon Backend

A Go-based backend API for the Avalon game, providing room management, player handling, real-time WebSocket events, and game state management.

## Features

- **Rooms** – Create and join rooms with unique 6-character codes, optional password, host/player roles
- **Games** – Start games from a room (host only); game engine with phases, votes, and actions
- **WebSockets** – Per-room lobby (`/ws/rooms/{code}`) and per-game (`/api/rooms/{code}/games/{game_id}/ws`) with token auth, chat, votes, and state sync
- **PostgreSQL** – Persistent storage with migrations (goose)
- **REST API** – RESTful endpoints with JSON; Swagger docs at `/docs`
- **Rate limiting** – Optional in-memory limiter (e.g. create/join/chat per IP)
- **Graceful shutdown** – Signal handling and server drain

## Prerequisites

- **Go 1.24** or later
- **PostgreSQL 12** or later
- **Make** (optional, for convenience commands)

## Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/vntrieu/avalon.git
   cd avalon
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Environment variables**
   
   Create a `.env` in the project root (e.g. from `.env.example` if present):
   ```env
   DATABASE_URL=postgres://user:password@localhost:5432/avalon?sslmode=disable
   AVALON_HTTP_ADDR=:8080
   MIGRATIONS_DIR=migrations
   WEBSOCKET_TOKEN_SECRET=your-secret-for-signing-ws-tokens
   ```
   If `WEBSOCKET_TOKEN_SECRET` is unset, a dev default is used (change in production).

4. **Create database and run migrations**
   ```bash
   createdb avalon
   go run ./cmd/server
   ```
   Migrations run automatically on startup.

## Running the application

**Development**
```bash
go run ./cmd/server
```

The server will connect to PostgreSQL, run pending migrations, and listen on `AVALON_HTTP_ADDR` (default `:8080`).

**Build and run**
```bash
go build -o avalon-server ./cmd/server
./avalon-server
```

## API overview

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/docs/` | Swagger UI; `/docs/doc.json` for OpenAPI spec |
| POST | `/api/rooms` | Create room (body: `display_name`, optional `password`, `settings`) |
| GET | `/api/rooms/{code}` | Get room by code |
| POST | `/api/rooms/{code}/join` | Join room (body: `display_name`, optional `password`) |
| POST | `/api/rooms/{code}/games` | Start a new game (host only; optional Bearer token or `room_player_id` in body) |
| GET | `/ws/rooms/{code}` | WebSocket for room lobby (token in query or cookie) |
| GET | `/api/rooms/{code}/games/{game_id}/ws` | WebSocket for game events |

Create/join room responses can include a WebSocket auth token when `WEBSOCKET_TOKEN_SECRET` is set. Use it as `?token=...` or `Authorization: Bearer <token>` for WebSocket connections.

## Project structure

```
avalon/
├── cmd/server/           # Entry point (main.go)
├── internal/
│   ├── auth/             # JWT-style tokens for WebSocket auth
│   ├── database/         # DB connection and goose migrations
│   ├── db/               # Generated SQL (sqlc) and models
│   ├── games/            # Game engine (phases, votes, actions)
│   ├── httpapi/          # Chi router, middleware, handlers
│   │   └── handler/      # Room, game, health handlers
│   ├── ratelimit/        # In-memory rate limiter
│   ├── store/            # Repository layer (rooms, games, events)
│   └── websocket/        # Hub, clients, event handler, game engine wiring
├── migrations/           # SQL migrations (goose)
├── docs/                 # Swag-generated API docs
├── go.mod
└── README.md
```

## Testing

```bash
go test ./...
```

With race detector and coverage:
```bash
go test -race -cover ./...
```

## Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | *required* |
| `AVALON_HTTP_ADDR` | HTTP listen address | `:8080` |
| `MIGRATIONS_DIR` | Directory for migration files | `migrations` |
| `WEBSOCKET_TOKEN_SECRET` | Secret for signing WebSocket auth tokens | dev default if unset |

## CI / CD

GitHub Actions (`.github/workflows/deploy.yml`):

- **CI** – On every push/PR: checkout, build `./cmd/server`, run tests with `-race`.
- **Deploy** – On push to `main` (non-PR): build Linux ARM64 binary, copy binary and `migrations/` to EC2 via SSH, restart the `avalon` systemd service, then smoke-check `http://EC2_HOST:8080/healthz`.

Required secrets: `EC2_HOST`, `EC2_SSH_KEY`, `DEPLOY_USER`, `DEPLOY_PATH`. See `plans/phase-08-deploy-ec2-github-actions.md` for setup.

## Technologies

- **Go 1.24** – Language and stdlib
- **PostgreSQL** – Database
- **pgx** – PostgreSQL driver
- **goose** – Migrations
- **chi** – HTTP router and middleware
- **gorilla/websocket** – WebSockets
- **swaggo** – Swagger/OpenAPI docs
- **golang.org/x/crypto** – bcrypt for password hashing

## License

[Add your license here]
