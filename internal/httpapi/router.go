package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/swaggo/http-swagger"
	"github.com/vntrieu/avalon/internal/httpapi/handler"
	"github.com/vntrieu/avalon/internal/ratelimit"
	"github.com/vntrieu/avalon/internal/store"
	"github.com/vntrieu/avalon/internal/websocket"

	_ "github.com/vntrieu/avalon/docs" // swag-generated docs
)

// NewRouter builds the root HTTP router with basic middleware and health check.
// tokenSecret is used to sign WebSocket auth tokens; if nil or empty, create/join responses omit the token.
// rateLimiter is optional: if nil, no rate limiting is applied; otherwise create room, join room, and WS chat are limited.
//
// @title            Avalon API
// @version          1.0
// @description      API for Avalon game rooms and games.
// @BasePath         /
// @SecurityDefinitions.apikey  BearerAuth
// @in               header
// @name             Authorization
func NewRouter(pool *pgxpool.Pool, tokenSecret []byte, rateLimiter ratelimit.Limiter) http.Handler {
	if rateLimiter == nil {
		rateLimiter = &ratelimit.Noop{}
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handler.Healthz)

	// Swagger UI and generated spec (from swag comments)
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})
	r.Get("/docs/*", httpSwagger.Handler(httpSwagger.URL("/docs/doc.json")))

	// Room and game stores (used by WS and routes)
	roomStore := store.NewRoomStore(pool)
	gameStore := store.NewGameStore(pool)
	engine := websocket.NewGameEngine(gameStore, pool)

	// Initialize WebSocket hub and handler (hub uses rateLimiter for chat)
	eventHandler := websocket.NewEventHandler(nil, pool, gameStore, engine, rateLimiter)
	hub := websocket.NewHub(eventHandler)
	eventHandler = websocket.NewEventHandler(hub, pool, gameStore, engine, rateLimiter)
	hub.SetEventHandler(eventHandler)
	go hub.Run()

	wsHandler := websocket.NewWSHandler(hub, pool, tokenSecret)

	// Per-room WebSocket (token auth, chat, vote, action, sync_state)
	r.Get("/ws/rooms/{code}", wsHandler.HandleRoomWebSocket)

	// Rate limit middleware for create/join (by IP)
	rateLimitByIP := RateLimitMiddleware(rateLimiter, RateLimitKeyByIP)

	// Room routes (body size limited to 1MB for JSON)
	roomHandler := handler.NewRoomHandler(roomStore, tokenSecret)
	r.Route("/api/rooms", func(r chi.Router) {
		r.Use(LimitRequestBody(DefaultMaxBodyBytes))
		r.With(rateLimitByIP).Post("/", roomHandler.CreateRoom)
		r.Get("/{code}", roomHandler.GetRoom)
		r.With(rateLimitByIP).Post("/{code}/join", roomHandler.JoinRoom)

		// Game routes (use room code, not room_id)
		gameHandler := handler.NewGameHandler(gameStore, roomStore, tokenSecret)
		r.Post("/{code}/games", gameHandler.CreateGame) // POST /api/rooms/{code}/games (host only)

		// WebSocket route for game events
		r.Get("/{code}/games/{game_id}/ws", wsHandler.HandleWebSocket)
	})

	return r
}

// DefaultRateLimiter returns an in-memory rate limiter for create/join/chat: 20 requests per minute per IP.
// Use in production or pass nil to disable. For multi-instance, replace with Redis-backed limiter.
func DefaultRateLimiter() ratelimit.Limiter {
	return ratelimit.NewInMemory(20, time.Minute)
}

// SetupRoomWSRouter returns a chi router with only GET /ws/rooms/{code} for testing.
func SetupRoomWSRouter(wsHandler *websocket.WSHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/ws/rooms/{code}", wsHandler.HandleRoomWebSocket)
	return r
}
