package websocket

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vntrieu/avalon/internal/auth"
	"github.com/vntrieu/avalon/internal/db"
	"github.com/vntrieu/avalon/internal/store"
)

// rateLimitKeyFromRequest returns a key for rate limiting (e.g. client IP).
func rateLimitKeyFromRequest(r *http.Request) string {
	if x := r.Header.Get("X-Real-IP"); x != "" {
		return x
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return x
	}
	return r.RemoteAddr
}

// WSHandler handles WebSocket connections (game and room).
type WSHandler struct {
	hub         *Hub
	pool        *pgxpool.Pool
	tokenSecret []byte
}

// NewWSHandler creates a new WSHandler. tokenSecret is used for room WS auth; if nil/empty, room WS rejects.
func NewWSHandler(hub *Hub, pool *pgxpool.Pool, tokenSecret []byte) *WSHandler {
	return &WSHandler{
		hub:         hub,
		pool:        pool,
		tokenSecret: tokenSecret,
	}
}

// HandleWebSocket handles WebSocket upgrade requests.
func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	gameID := chi.URLParam(r, "game_id")
	roomPlayerID := r.URL.Query().Get("room_player_id")

	if code == "" || gameID == "" {
		http.Error(w, "code and game_id are required", http.StatusBadRequest)
		return
	}

	// Resolve room code to room_id
	queries := db.New(h.pool)
	roomRow, err := queries.GetRoomByCode(r.Context(), code)
	if err != nil {
		log.Printf("websocket: room not found for code %q: %v", code, err)
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	roomID := pgtypeUUIDToString(roomRow.ID)

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	// Use Background so event handling is not tied to the HTTP request lifecycle.
	// The request context is canceled when the handler returns after the upgrade.
	client := &Client{
		hub:          h.hub,
		conn:         conn,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       roomID,
		GameID:       gameID,
		RoomPlayerID: roomPlayerID,
		ctx:          context.Background(),
	}

	client.hub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// HandleRoomWebSocket handles GET /ws/rooms/{code} with token auth. Client sends token via query param or Authorization header.
func (h *WSHandler) HandleRoomWebSocket(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		const prefix = "Bearer "
		if v := r.Header.Get("Authorization"); strings.HasPrefix(v, prefix) {
			token = strings.TrimSpace(v[len(prefix):])
		}
	}
	if token == "" || len(h.tokenSecret) == 0 {
		h.rejectRoomWS(w, r, "missing or invalid token")
		return
	}
	claims, err := auth.VerifyToken(token, h.tokenSecret)
	if err != nil {
		log.Printf("websocket room auth: code=%s token verification failed: %v", code, err)
		h.rejectRoomWS(w, r, "unauthorized")
		return
	}
	queries := db.New(h.pool)
	roomRow, err := queries.GetRoomByCode(r.Context(), code)
	if err != nil {
		log.Printf("websocket room: room not found for code %q: %v", code, err)
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	roomID := pgtypeUUIDToString(roomRow.ID)
	if roomID != claims.RoomID {
		h.rejectRoomWS(w, r, "room does not match token")
		return
	}
	roomStore := store.NewRoomStore(h.pool)
	roomPlayer, err := roomStore.GetRoomPlayerInRoom(r.Context(), code, claims.RoomPlayerID)
	if err != nil {
		log.Printf("websocket room: code=%s room_id=%s player_id=%s player not in room: %v", code, roomID, claims.RoomPlayerID, err)
		h.rejectRoomWS(w, r, "player not in room")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket room upgrade error: %v", err)
		return
	}
	client := &Client{
		hub:          h.hub,
		conn:         conn,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       roomID,
		GameID:       "",
		RoomPlayerID: roomPlayer.ID,
		DisplayName:  roomPlayer.DisplayName,
		RateLimitKey: rateLimitKeyFromRequest(r),
		ctx:          context.Background(),
	}
	client.hub.register <- client
	go client.writePump()
	go client.readPump()
}

// rejectRoomWS responds with 401 before upgrade (auth is always checked before upgrading).
func (h *WSHandler) rejectRoomWS(w http.ResponseWriter, _ *http.Request, reason string) {
	http.Error(w, reason, http.StatusUnauthorized)
}

// pgtypeUUIDToString converts pgtype.UUID to standard UUID string.
func pgtypeUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}
