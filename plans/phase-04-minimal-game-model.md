# Phase 4: Implement minimal game model

**Goal**: Implement minimal game creation and association with rooms, plus initial game state snapshotting. Expose “start new game” so the host can create a fresh game in a room.

See **avalon-go-backend-master.md** for Start new game endpoint and game/room lifecycle.

---

## Summary

- Use existing `games`, `game_players`, and `game_state_snapshots` tables.
- Implement store methods: create game for a room, add game_players, create/update snapshot (versioned).
- **POST /api/rooms/{code}/games** — host only; creates a new game in the room with given rules preset; previous games remain in history.
- Ensure GET /api/rooms/{code} continues to return the “latest” game (most recent by creation or status) and its latest snapshot.

---

## Concrete steps

1. **Store layer**
   - **CreateGame(roomID, config)** — insert `games` with status `waiting`, `config_json` from preset or request.
   - **CreateGamePlayer(gameID, roomPlayerID)** — insert into `game_players` (role/slot can be null until assignment).
   - **CreateOrUpdateSnapshot(gameID, version, stateJSON)** — insert into `game_state_snapshots` with next version; ensure one row per (game_id, version) or upsert latest.
   - **GetLatestGameForRoom(roomID)** — return game with highest `created_at` (or filter by status).
   - **GetLatestSnapshotForGame(gameID)** — return snapshot with max `version` for that game.

2. **Game creation on room create/join**
   - When creating a room (Phase 3), ensure an initial game is created (already in Phase 3 scope) and optionally an initial snapshot (e.g. empty state or lobby state).
   - When joining, association with “latest game” and `game_players` row is already part of Phase 3; no change required if already implemented.

3. **Start new game endpoint**
   - **POST /api/rooms/{code}/games**
   - Auth: caller must be host (e.g. pass `room_player_id` or token; verify `is_host` for that room).
   - Body (optional): rule preset name or `config_json` override.
   - In a transaction: create new `games` row for room (status `waiting`), create `game_players` for all current room_players (or only those still in room), create initial snapshot (e.g. version 1, lobby state).
   - Response: 201 with game `id`, status, and optionally latest snapshot.

4. **Latest game semantics**
   - “Latest” = most recently created game for the room (or explicitly “current” if you add such a flag). GET /api/rooms/{code} must return this game and its latest snapshot so clients always see the current game after join/reload.

5. **Config and status**
   - `config_json`: store rules preset (roles, phases, victory conditions) as JSON; can be a known struct or generic map.
   - Game status: `waiting` (lobby), `in_progress` (once engine starts), `finished` (set in Phase 6).

---

## References

- `internal/store/game.go` — create game, snapshot, get latest game/snapshot.
- `internal/httpapi/game_handler.go` — POST /api/rooms/{code}/games handler.
- `internal/httpapi/router.go` — register games route.
- `sql/queries/games.sql` — sqlc queries for games and snapshots if used.

---

## Acceptance criteria

- [ ] Creating a room creates an initial game and optionally an initial snapshot.
- [ ] POST /api/rooms/{code}/games (as host) creates a new game and initial snapshot; previous games remain.
- [ ] GET /api/rooms/{code} returns the latest game and its latest snapshot.
- [ ] Non-host POST to /api/rooms/{code}/games returns 403 (or 401 if no auth).
