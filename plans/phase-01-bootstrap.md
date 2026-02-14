# Phase 1: Bootstrap project & dependencies

**Goal**: Bootstrap Go module, server entrypoint, HTTP router, config, and logging so the app can start and serve basic HTTP.

See **avalon-go-backend-master.md** for full architecture and project structure.

---

## Summary

- Create Go module and minimal dependency set.
- Implement `cmd/server/main.go` as the single entry point.
- Load configuration from environment variables.
- Set up HTTP router (Go 1.22+ `ServeMux` or lightweight router).
- Add middleware: logging, CORS, panic recovery, optional request ID.
- Expose a health/ping endpoint for readiness checks.
- Implement graceful shutdown on SIGINT/SIGTERM.

---

## Concrete steps

1. **Go module**
   - Ensure `go.mod` exists with module path (e.g. `github.com/yourorg/avalon` or local path).
   - Add only required dependencies (e.g. `pgx`, `gorilla/websocket`, bcrypt/argon2) in later phases; keep this phase minimal.

2. **Configuration**
   - Read from env: `PORT` (default 8080), `DATABASE_URL` (optional for this phase), `LOG_LEVEL` (optional).
   - Use a small config struct and `os.Getenv` or a minimal env loader.

3. **Entry point** (`cmd/server/main.go`)
   - Parse config.
   - Initialize logger (stdlib `log` or structured logger).
   - Create HTTP server with read/write timeouts.
   - Register router (e.g. `http.NewServeMux()` with method-based routes).
   - Register middleware chain: recovery → request ID → logging → CORS → router.
   - Start server in a goroutine; block on `ListenAndServe` or `Server.ListenAndServeTLS` if needed.
   - On shutdown signal, call `Server.Shutdown(ctx)` with a timeout (e.g. 10s).

4. **Router**
   - Mount routes under `/api` for future REST (e.g. `/api/rooms`).
   - Add `GET /health` or `GET /ping` returning 200 and optionally `{"status":"ok"}`.

5. **Middleware**
   - **Recovery**: recover panics, log, return 500.
   - **Request ID**: add/generate `X-Request-ID` for tracing.
   - **Logging**: log method, path, remote addr, request ID, duration, status code.
   - **CORS**: set `Access-Control-Allow-Origin` (and methods/headers if needed) for browser clients.

---

## References

- `cmd/server/main.go` — entry point, config, server, shutdown.
- `internal/httpapi/` or `internal/http/` — router, handlers, middleware (e.g. `router.go`, `middleware.go`).

---

## Acceptance criteria

- [ ] `go build ./cmd/server` succeeds.
- [ ] Server starts and listens on configured port.
- [ ] `GET /health` (or `/ping`) returns 200.
- [ ] Panic in a handler returns 500 and is logged; server keeps running.
- [ ] Graceful shutdown completes within timeout when receiving SIGINT/SIGTERM.
