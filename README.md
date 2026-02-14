# Avalon Backend

A Go-based backend API for the Avalon game, providing room management, player handling, and game state management.

## Features

- Room creation and management with unique room codes
- Password-protected rooms
- Player management within rooms
- PostgreSQL database with migrations
- RESTful API endpoints
- Graceful server shutdown

## Prerequisites

- Go 1.24.0 or later
- PostgreSQL 12 or later
- Make (optional, for convenience commands)

## Setup

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd avalon
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set up environment variables**
   
   Create a `.env` file in the root directory:
   ```env
   DATABASE_URL=postgres://user:password@localhost:5432/avalon?sslmode=disable
   AVALON_HTTP_ADDR=:8080
   MIGRATIONS_DIR=migrations
   ```

4. **Set up PostgreSQL database**
   ```bash
   createdb avalon
   ```

5. **Run migrations**
   
   Migrations run automatically when the server starts, or you can run them manually using goose:
   ```bash
   go run cmd/server/main.go
   ```

## Running the Application

### Development

```bash
go run cmd/server/main.go
```

The server will:
- Connect to PostgreSQL
- Run pending migrations automatically
- Start listening on `:8080` (or the port specified in `AVALON_HTTP_ADDR`)

### Build and Run

```bash
go build -o avalon cmd/server/main.go
./avalon
```

## API Endpoints

### Health Check
- `GET /healthz` - Health check endpoint

### Rooms
- `POST /api/rooms` - Create a new room
  ```json
  {
    "display_name": "Player Name",
    "password": "optional-password",
    "settings": {}
  }
  ```

## Project Structure

```
avalon/
├── cmd/
│   └── server/          # Application entry point
│       └── main.go
├── internal/
│   ├── database/        # Database connection and migrations
│   │   ├── database.go
│   │   └── migrate.go
│   ├── httpapi/         # HTTP handlers and routing
│   │   ├── router.go
│   │   └── room_handler.go
│   └── store/           # Data access layer (repository pattern)
│       └── room.go
├── migrations/          # Database migration files
│   └── 20240213000000_init.sql
├── go.mod
├── go.sum
└── README.md
```

## Testing

Run tests:
```bash
go test ./...
```

Run tests with coverage:
```bash
go test -cover ./...
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Required |
| `AVALON_HTTP_ADDR` | HTTP server address | `:8080` |
| `MIGRATIONS_DIR` | Directory containing migration files | `migrations` |

## Database Migrations

Migrations are managed using [goose](https://github.com/pressly/goose). Migration files are located in the `migrations/` directory and are automatically applied when the server starts.

## Technologies

- **Go 1.24+** - Programming language
- **PostgreSQL** - Database
- **pgx** - PostgreSQL driver
- **goose** - Database migrations
- **chi** - HTTP router
- **bcrypt** - Password hashing

## License

[Add your license here]
