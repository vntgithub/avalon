# Phase 9: Users and user info storage

**Goal**: Introduce persistent user accounts so the backend can store user info (profile, identity across rooms). Anonymous play remains supported: when a user is logged in and creates or joins a room, we associate that room player with their user; when not logged in, behavior stays as today (no user link).

---

## Summary

- **Users table**: Store user identity (id, email, password_hash, display_name, created_at, updated_at). Optional: avatar_url, settings_json.
- **Link room_players to users**: Add nullable `user_id` on `room_players`. When create/join is called with a valid user session token, set `room_players.user_id`; otherwise leave null (anonymous).
- **Auth**: Register (POST /api/auth/register), Login (POST /api/auth/login) returning a **user session token** (HMAC with user_id + exp). Reuse existing HMAC approach in internal/auth/token.go with UserClaims and helpers: GenerateUserToken, VerifyUserToken.
- **Me endpoint**: GET /api/users/me — return current user profile when Authorization Bearer is a valid user session token; 401 otherwise.
- **Create/join room**: Accept optional Bearer user token. If present and valid, set `room_players.user_id` when creating or joining; response still includes the existing room-scoped WebSocket token for WS auth.

---

## References

- migrations/20250214000000_add_users.sql — users table and room_players.user_id.
- internal/auth/token.go — user token type and VerifyUserToken.
- internal/store/room.go — CreateRoom, JoinRoom; optional userID passed to DB.
- internal/store/user.go — UserStore (CreateUser, GetUserByEmail, GetUserByID).
- internal/httpapi/handler/auth.go — register, login, GetMe.
- internal/httpapi/handler/room.go — create/join read user from context, pass to store.
- internal/httpapi/middleware.go — OptionalUser, RequireUser.

---

## Acceptance criteria

- [ ] Migration adds `users` table and `room_players.user_id`; app starts and migrations run.
- [ ] POST /api/auth/register creates user and returns user session token; duplicate email returns 409.
- [ ] POST /api/auth/login returns user session token for valid email/password; invalid returns 401.
- [ ] GET /api/users/me returns current user when Bearer user token is valid; 401 without/invalid token.
- [ ] Creating a room with valid user Bearer token sets room_players.user_id for the host; without token, user_id remains null.
- [ ] Joining a room with valid user Bearer token sets room_players.user_id for the joining player; without token, user_id remains null.
- [ ] Existing create/join flows without user token still work (anonymous play unchanged).
- [ ] Swagger/docs updated for new endpoints.
