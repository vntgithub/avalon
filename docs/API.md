# API docs

API reference is served as **Swagger UI** at `GET /docs` when the server is running. Use that for endpoints, request/response schemas, and try-it-out.

## Generating Swagger docs

Regenerate the Swagger spec and UI after adding or changing API handlers or their swag comments.

1. **Install swag** (once):
   ```bash
   go install github.com/swaggo/swag/cmd/swag@latest
   ```

2. **From the project root**, run:
   ```bash
   GOFLAGS=-mod=mod swag init -g internal/httpapi/router.go -o docs --parseInternal --parseDependency
   ```

   This updates `docs/docs.go`, `docs/swagger.json`, and `docs/swagger.yaml`. The server serves the spec at `/docs/doc.json` and the Swagger UI at `/docs/`.

3. **When to re-run:** After changing `@Summary`, `@Param`, `@Router`, `@Success`, `@Failure`, or other swag annotations in `internal/httpapi/router.go` or `internal/httpapi/handler/*.go`, or when adding new HTTP handlers that should appear in the docs.
