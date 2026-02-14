# Phase 2: Set up PostgreSQL & migrations

**Goal**: Define PostgreSQL schema via migrations and wire the store layer (pgx/sqlc) so the app can run migrations and execute queries.

See **avalon-go-backend-master.md** for full database schema design.

---

## Summary

- Add a migration tool (e.g. `golang-migrate`) and create migration files under `migrations/`.
- Implement all tables: `rooms`, `room_players`, `games`, `game_players`, `game_state_snapshots`, `moves`/`events`, optionally `chat_messages`.
- Create `internal/store` (and `internal/db` if using sqlc) to run queries and transactions.
- Wire DB connection pool from `main.go` and run migrations on startup (or via CLI).

---

## Concrete steps

1. **Migration tool**
   - Add `golang-migrate/migrate` (or similar) to the project.
   - Create `migrations/` directory with versioned files (e.g. `000001_create_rooms.up.sql`, `000001_create_rooms.down.sql`).

2. **Schema migrations** (match master plan)
   - **rooms**: `id` UUID PRIMARY KEY, `code` UNIQUE NOT NULL, `password_hash` TEXT, `created_at`, `updated_at`, `settings_json` JSONB.
   - **room_players**: `id` UUID PRIMARY KEY, `room_id` FK, `display_name`, `is_host` BOOLEAN, `created_at`.
   - **games**: `id` UUID PRIMARY KEY, `room_id` FK, `status` (e.g. `waiting`/`in_progress`/`finished`), `created_at`, `ended_at`, `config_json` JSONB.
   - **game_players**: `id` UUID PRIMARY KEY, `game_id` FK, `room_player_id` FK, role/slot fields, timestamps.
   - **game_state_snapshots**: `id` UUID PRIMARY KEY, `game_id` FK, `version` INT, `state_json` JSONB, `created_at`.
   - **moves** or **events**: `id` UUID PRIMARY KEY, `game_id` FK, `room_player_id` FK, `type` TEXT, `payload_json` JSONB, `created_at`.
   - **chat_messages** (optional): `id`, `room_id` or `game_id`, `room_player_id`, `message`, `created_at`.
   - Add indexes for common lookups (e.g. room by `code`, game by `room_id`, snapshot by `game_id` + `version`).

3. **Store layer**
   - Use **pgx** with hand-written SQL or **sqlc** for type-safe queries.
   - If sqlc: add `sql/queries/*.sql` and generate `internal/db/*.go`; expose a `Querier` or store interface.
   - Implement: create/get room, create/get room_players, create/get game, create/get snapshots, append move/event.
   - Support transactions for multi-table operations (e.g. create room + host player + initial game).

4. **Wire from main**
   - Parse `DATABASE_URL` and create pgx pool.
   - Run migrations on startup (or provide a separate `migrate up` command).
   - Pass pool or store into HTTP handler constructors (in later phases).

---

## References

- `migrations/` — SQL migration files.
- `internal/store/` — Go API for DB (e.g. `room.go`, `game.go`).
- `internal/db/` — sqlc-generated code and `querier.go` if using sqlc.
- `sql/queries/` — sqlc query files (e.g. `rooms.sql`, `games.sql`).

---

## Acceptance criteria

- [ ] `migrate up` (or equivalent) creates all tables and indexes.
- [ ] `migrate down` (or equivalent) drops them cleanly.
- [ ] Store layer can create a room, a room_player, and a game in a transaction.
- [ ] Store can fetch room by code and latest game for a room.
- [ ] Server starts successfully with valid `DATABASE_URL` and runs migrations.
