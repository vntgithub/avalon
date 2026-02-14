package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wsgorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vntrieu/avalon/internal/auth"
	"github.com/vntrieu/avalon/internal/httpapi"
	"github.com/vntrieu/avalon/internal/store"
	"github.com/vntrieu/avalon/internal/websocket"
)

// TestRoomWebSocket_Unauthorized verifies that GET /ws/rooms/{code} without a valid token returns 401.
func TestRoomWebSocket_Unauthorized(t *testing.T) {
	pool := store.SetupTestDB(t)
	defer pool.Close()

	roomStore := store.NewRoomStore(pool)
	createResp, err := roomStore.CreateRoom(context.Background(), store.CreateRoomRequest{DisplayName: "Host"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	code := createResp.Room.Code
	if code == "" {
		t.Fatal("room code empty")
	}

	tokenSecret := []byte("test-secret")
	gameStore := store.NewGameStore(pool)
	eventHandler := websocket.NewEventHandler(nil, pool, gameStore, nil, nil)
	hub := websocket.NewHub(eventHandler)
	eventHandler = websocket.NewEventHandler(hub, pool, gameStore, nil, nil)
	hub.SetEventHandler(eventHandler)
	go hub.Run()
	wsHandler := websocket.NewWSHandler(hub, pool, tokenSecret)
	router := httpapi.SetupRoomWSRouter(wsHandler)

	// No token -> 401
	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/"+code, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}

	// Invalid token -> 401
	req2 := httptest.NewRequest(http.MethodGet, "/ws/rooms/"+code+"?token=invalid", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", w2.Code)
	}
}

// setupRoomWSWithEngine returns router, room code, and a token for the host (room_id, room_player_id from createResp).
func setupRoomWSWithEngine(t *testing.T) (http.Handler, string, string, *pgxpool.Pool) {
	t.Helper()
	pool := store.SetupTestDB(t)
	roomStore := store.NewRoomStore(pool)
	createResp, err := roomStore.CreateRoom(context.Background(), store.CreateRoomRequest{DisplayName: "Host"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	code := createResp.Room.Code
	if code == "" {
		t.Fatal("room code empty")
	}
	tokenSecret := []byte("test-secret")
	gameStore := store.NewGameStore(pool)
	engine := websocket.NewGameEngine(gameStore, pool)
	eventHandler := websocket.NewEventHandler(nil, pool, gameStore, engine, nil)
	hub := websocket.NewHub(eventHandler)
	eventHandler = websocket.NewEventHandler(hub, pool, gameStore, engine, nil)
	hub.SetEventHandler(eventHandler)
	go hub.Run()
	wsHandler := websocket.NewWSHandler(hub, pool, tokenSecret)
	router := httpapi.SetupRoomWSRouter(wsHandler)
	token, _, err := auth.GenerateToken(createResp.Room.ID, createResp.RoomPlayer.ID, tokenSecret, auth.DefaultTokenExpiry)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return router, code, token, pool
}

// serverWSURL converts httptest.Server URL to ws URL.
func serverWSURL(server *httptest.Server, path string) string {
	return "ws" + server.URL[4:] + path
}

// TestRoomWebSocket_ValidToken_SyncState connects with valid token, sends sync_state, asserts state envelope.
func TestRoomWebSocket_ValidToken_SyncState(t *testing.T) {
	router, code, token, pool := setupRoomWSWithEngine(t)
	defer pool.Close()
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := serverWSURL(server, "/ws/rooms/"+code+"?token="+token)
	conn, _, err := wsgorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send sync_state
	msg := map[string]interface{}{"type": "sync_state"}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read state envelope (type "state", event "state")
	var envelope struct {
		Type    string                 `json:"type"`
		Event   string                 `json:"event"`
		Payload map[string]interface{} `json:"payload"`
	}
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("read: %v", err)
	}
	if envelope.Type != "state" {
		t.Errorf("expected envelope type state, got %s", envelope.Type)
	}
	if envelope.Event != "state" {
		t.Errorf("expected event state, got %s", envelope.Event)
	}
	if envelope.Payload["game_id"] == nil && envelope.Payload["state"] == nil {
		t.Error("expected payload with game_id or state")
	}
}

// TestRoomWebSocket_ChatBroadcast: two clients connect; first sends chat, second receives broadcast.
func TestRoomWebSocket_ChatBroadcast(t *testing.T) {
	router, code, hostToken, pool := setupRoomWSWithEngine(t)
	defer pool.Close()
	// Add second player and get token
	roomStore := store.NewRoomStore(pool)
	joinResp, err := roomStore.JoinRoom(context.Background(), store.JoinRoomRequest{
		Code:        code,
		DisplayName: "Player2",
	})
	if err != nil {
		t.Fatalf("join room: %v", err)
	}
	tokenSecret := []byte("test-secret")
	player2Token, _, err := auth.GenerateToken(joinResp.Room.ID, joinResp.RoomPlayer.ID, tokenSecret, auth.DefaultTokenExpiry)
	if err != nil {
		t.Fatalf("generate token player2: %v", err)
	}

	server := httptest.NewServer(router)
	defer server.Close()

	// Connect host
	conn1, _, err := wsgorilla.DefaultDialer.Dial(serverWSURL(server, "/ws/rooms/"+code+"?token="+hostToken), nil)
	if err != nil {
		t.Fatalf("dial host: %v", err)
	}
	defer conn1.Close()

	// Connect player2
	conn2, _, err := wsgorilla.DefaultDialer.Dial(serverWSURL(server, "/ws/rooms/"+code+"?token="+player2Token), nil)
	if err != nil {
		t.Fatalf("dial player2: %v", err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	// Host sends chat
	chatMsg := map[string]interface{}{"type": "chat", "payload": map[string]interface{}{"message": "hello room"}}
	if err := conn1.WriteJSON(chatMsg); err != nil {
		t.Fatalf("write chat: %v", err)
	}

	// Player2 should receive chat event (broadcast to room except sender)
	var envelope struct {
		Type    string                 `json:"type"`
		Event   string                 `json:"event"`
		Payload map[string]interface{} `json:"payload"`
	}
	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn2.ReadJSON(&envelope); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	if envelope.Type != "event" {
		t.Errorf("expected type event, got %s", envelope.Type)
	}
	if envelope.Event != "chat" {
		t.Errorf("expected event chat, got %s", envelope.Event)
	}
	if envelope.Payload["message"] != "hello room" {
		t.Errorf("expected message hello room, got %v", envelope.Payload["message"])
	}
}

// TestRoomWebSocket_ReconnectSyncState: connect, disconnect, reconnect with same token, send sync_state, assert state.
func TestRoomWebSocket_ReconnectSyncState(t *testing.T) {
	router, code, token, pool := setupRoomWSWithEngine(t)
	defer pool.Close()
	server := httptest.NewServer(router)
	defer server.Close()
	wsURL := serverWSURL(server, "/ws/rooms/"+code+"?token="+token)

	// First connection
	conn1, _, err := wsgorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	conn1.WriteJSON(map[string]string{"type": "sync_state"})
	var firstEnvelope struct {
		Type    string                 `json:"type"`
		Payload map[string]interface{} `json:"payload"`
	}
	conn1.ReadJSON(&firstEnvelope)
	conn1.Close()

	time.Sleep(20 * time.Millisecond)

	// Reconnect with same token
	conn2, _, err := wsgorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 2 (reconnect): %v", err)
	}
	defer conn2.Close()

	conn2.WriteJSON(map[string]string{"type": "sync_state"})
	var secondEnvelope struct {
		Type    string                 `json:"type"`
		Payload map[string]interface{} `json:"payload"`
	}
	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn2.ReadJSON(&secondEnvelope); err != nil {
		t.Fatalf("read after reconnect: %v", err)
	}
	if secondEnvelope.Type != "state" {
		t.Errorf("expected type state after reconnect, got %s", secondEnvelope.Type)
	}
	// Same room/game so payload should be consistent (e.g. both have phase)
	if secondEnvelope.Payload["phase"] == nil && secondEnvelope.Payload["state"] == nil {
		t.Error("expected payload with phase or state after sync_state")
	}
}
