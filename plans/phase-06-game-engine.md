# Phase 6: Extend game engine

**Goal**: Implement the full game engine: configurable phases, roles, and actions; state machine; validation; persistence via moves/events and game_state_snapshots. Wire WebSocket `vote` and `action` messages to the engine and broadcast resulting state/events.

See **avalon-go-backend-master.md** for Game engine & rules configuration and Room & game lifecycle.

---

## Summary

- **Rules config**: Presets (e.g. classic Avalon) as JSON/structs; `config_json` in DB. Phases have name, allowed actions, constraints.
- **State machine**: Single state struct (current phase, round, leader, teams, mission results, player roles, etc.). Apply moves → new state + side effects (new snapshot, broadcast).
- **Validation**: Only legal actions for current phase; only eligible actors (e.g. leader proposes team, players vote).
- **Persistence**: Append to moves/events table; write game_state_snapshots at key points (e.g. once per phase or after each vote).
- **WebSocket**: Handle `vote` and `action` message types; call engine; broadcast `state` and `event` messages; support `sync_state` (reply with latest snapshot).

---

## Concrete steps

1. **Rules configuration**
   - Define preset(s) in code or JSON: list of phases (e.g. `team_selection`, `mission_vote`, `mission_resolution`), allowed action types per phase, constraints (team size, timeouts).
   - Load into game from `config_json` when starting or loading game. Engine uses this to decide allowed actions and transitions.

2. **State struct**
   - Hold: game_id, current_phase, round_index, leader_player_id, proposed_team_ids, mission_votes, mission_results, player_roles (or role_ids per player), status.
   - Serialize to/from JSON for snapshots. Version snapshots by incrementing version number per write.

3. **Engine API**
   - **ApplyMove(gameID, roomPlayerID, moveType, payload)** (or **HandleAction**): validate move against current state and rules (phase, actor, payload shape). If valid: compute new state, append event to DB, write new snapshot, return new state and list of events to broadcast.
   - **GetState(gameID)** — load latest snapshot for game; deserialize to state struct (or replay events if no snapshot).

4. **Validation**
   - Per phase: only certain actions allowed (e.g. in `team_selection` only leader can `propose_team`, then everyone can `approve_team` / `reject_team`).
   - Check actor is the right player (e.g. leader), payload matches (e.g. team size, player ids in room). Return clear error for invalid moves.

5. **Transitions**
   - Define phase transition rules (e.g. after team approved → `mission_vote`; after all votes in → `mission_resolution`; after resolution → next round or game end).
   - On game end (good/evil win condition), set game status to `finished` and set `ended_at`; optionally write final snapshot.

6. **WebSocket integration**
   - On `vote` message: parse payload (e.g. mission vote yes/no), call engine ApplyMove with type `vote`. On success: broadcast `event` (e.g. `vote_recorded`) and optionally `state` (full or delta). On failure: send `error` to client.
   - On `action` message: parse payload (e.g. propose_team, approve_team), call engine ApplyMove with type `action`. Same broadcast/error handling.
   - On `sync_state`: load latest snapshot for the game the client is in; send `state` message with snapshot. No persistence change.

7. **Idempotency / ordering**
   - Optionally include a client-side sequence or id in payload to deduplicate; or rely on server-generated event ids and reject duplicates if needed.

---

## References

- `internal/games/` or game engine package — state struct, rules config, ApplyMove, validation, phase transitions.
- `internal/store/` — persist events/moves, read/write snapshots, update game status/ended_at.
- `internal/websocket/ws_handler.go` — dispatch `vote`/`action` to engine; broadcast `state`/`event`; handle `sync_state`.

---

## Acceptance criteria

- [ ] Engine applies valid moves and advances phase/state; invalid moves return clear errors.
- [ ] Each significant state change produces an event row and a new or updated snapshot.
- [ ] WebSocket `vote` and `action` messages are validated and applied; results broadcast to room.
- [ ] `sync_state` returns latest snapshot for the client’s game.
- [ ] Game can transition to `finished` and `ended_at` is set when victory condition is met.
