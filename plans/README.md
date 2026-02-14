# Avalon backend — plans

This folder holds the master plan and phase-by-phase implementation plans for the Avalon Go + PostgreSQL backend.

## Master plan

- **[avalon-go-backend-master.md](avalon-go-backend-master.md)** — Full design: architecture, schema, use cases, WebSocket design, game engine, security, testing, and suggested implementation order.

## Phases (recommended order)

| Phase | File | Summary |
|-------|------|--------|
| 1 | [phase-01-bootstrap.md](phase-01-bootstrap.md) | Bootstrap Go module, server entrypoint, HTTP router, config, logging |
| 2 | [phase-02-postgres-migrations.md](phase-02-postgres-migrations.md) | PostgreSQL schema migrations and store layer (pgx/sqlc) |
| 3 | [phase-03-room-endpoints.md](phase-03-room-endpoints.md) | REST: create room, join room, get room/latest game |
| 4 | [phase-04-minimal-game-model.md](phase-04-minimal-game-model.md) | Minimal game model and start-new-game endpoint |
| 5 | [phase-05-websocket-hub.md](phase-05-websocket-hub.md) | Per-room WebSocket hub, auth, chat broadcast |
| 6 | [phase-06-game-engine.md](phase-06-game-engine.md) | Full game engine: phases, roles, actions, persistence, vote/action over WS |
| 7 | [phase-07-polish-harden.md](phase-07-polish-harden.md) | Security, validation, rate limiting, unit and integration tests |
| 8 | [phase-08-deploy-ec2-github-actions.md](phase-08-deploy-ec2-github-actions.md) | Deploy to EC2 via GitHub Actions CI/CD (build, test, SSH deploy, systemd) |

Work through phases 1–7 for the app; phase 8 adds deploy to EC2 using GitHub Actions.
