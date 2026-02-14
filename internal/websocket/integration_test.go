package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vntrieu/avalon/internal/db"
	"github.com/vntrieu/avalon/internal/store"
)


// createTestGameEvent creates a test game event
func createTestGameEvent(gameID, eventType string, payload map[string]interface{}) *store.GameEvent {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	return &store.GameEvent{
		ID:        "test-event-id",
		GameID:    gameID,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
}

func TestWebSocketConnection(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract room_id and game_id from URL
		roomID := "room-1"
		gameID := "game-1"
		roomPlayerID := "player-1"

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade error: %v", err)
		}

		client := &Client{
			hub:          hub,
			conn:         conn,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       roomID,
			GameID:       gameID,
			RoomPlayerID: roomPlayerID,
			ctx:          r.Context(),
		}

		hub.register <- client
		go client.writePump()
		go client.readPump()
	}))
	defer server.Close()

	// Connect to WebSocket
	wsURL := "ws" + server.URL[4:] // Convert http to ws
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial error: %v", err)
	}
	defer conn.Close()

	// Give time for registration
	time.Sleep(10 * time.Millisecond)

	// Verify client is registered
	count := hub.GetRoomClientCount("room-1")
	if count != 1 {
		t.Errorf("expected 1 client in room, got %d", count)
	}
}

func TestWebSocketEventSending(t *testing.T) {
	pool := store.SetupTestDB(t)
	defer pool.Close()

	queries := db.New(pool)
	gameStore := store.NewGameStore(pool)
	eventHandler := NewEventHandler(nil, pool, gameStore, nil, nil)
	hub := NewHub(eventHandler)
	eventHandler = NewEventHandler(hub, pool, gameStore, nil, nil)
	hub.SetEventHandler(eventHandler)
	go hub.Run()

	// Create room and game for testing
	roomStore := store.NewRoomStore(pool)
	ctx := context.Background()

	// Create room
	createRoomReq := store.CreateRoomRequest{}
	roomResp, err := roomStore.CreateRoom(ctx, createRoomReq, "TestPlayer", nil)
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	// Create game
	createGameReq := store.CreateGameRequest{
		RoomID: roomResp.Room.ID,
	}
	gameResp, err := gameStore.CreateGame(ctx, createGameReq)
	if err != nil {
		t.Fatalf("failed to create game: %v", err)
	}

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roomID := roomResp.Room.ID
		gameID := gameResp.Game.ID
		roomPlayerID := roomResp.RoomPlayer.ID

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade error: %v", err)
		}

		client := &Client{
			hub:          hub,
			conn:         conn,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       roomID,
			GameID:       gameID,
			RoomPlayerID: roomPlayerID,
			ctx:          r.Context(),
		}

		hub.register <- client
		go client.writePump()
		go client.readPump()
	}))
	defer server.Close()

	// Connect to WebSocket
	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial error: %v", err)
	}
	defer conn.Close()

	// Give time for registration
	time.Sleep(50 * time.Millisecond)

	// Send an event
	eventReq := store.CreateGameEventRequest{
		GameID: gameResp.Game.ID,
		Type:   "test_event",
		Payload: map[string]interface{}{
			"message": "test message",
		},
	}

	eventJSON, err := json.Marshal(eventReq)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, eventJSON); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// Give time for event processing
	time.Sleep(100 * time.Millisecond)

	// Verify event was created in database
	gameUUID, err := stringToUUID(gameResp.Game.ID)
	if err != nil {
		t.Fatalf("failed to convert game ID: %v", err)
	}

	events, err := queries.GetGameEventsByGameId(ctx, gameUUID)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Error("expected at least one event in database")
	} else {
		if events[0].Type != "test_event" {
			t.Errorf("expected event type 'test_event', got %s", events[0].Type)
		}
	}
}

func TestWebSocketBroadcastToMultipleClients(t *testing.T) {
	pool := store.SetupTestDB(t)
	defer pool.Close()

	queries := db.New(pool)
	gameStore := store.NewGameStore(pool)
	eventHandler := NewEventHandler(nil, pool, gameStore, nil, nil)
	hub := NewHub(eventHandler)
	eventHandler = NewEventHandler(hub, pool, gameStore, nil, nil)
	hub.SetEventHandler(eventHandler)
	go hub.Run()

	// Create room and game
	roomStore := store.NewRoomStore(pool)
	ctx := context.Background()

	createRoomReq := store.CreateRoomRequest{}
	roomResp, err := roomStore.CreateRoom(ctx, createRoomReq, "TestPlayer", nil)
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	// Add another player
	joinReq := store.JoinRoomRequest{
		Code: roomResp.Room.Code,
	}
	joinResp, err := roomStore.JoinRoom(ctx, joinReq, "Player2", nil)
	if err != nil {
		t.Fatalf("failed to join room: %v", err)
	}

	// Create game
	createGameReq := store.CreateGameRequest{
		RoomID: roomResp.Room.ID,
	}
	gameResp, err := gameStore.CreateGame(ctx, createGameReq)
	if err != nil {
		t.Fatalf("failed to create game: %v", err)
	}

	// Create test server
	receivedEvents := make(map[string][]*store.GameEvent)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roomID := roomResp.Room.ID
		gameID := gameResp.Game.ID
		roomPlayerID := r.URL.Query().Get("room_player_id")
		if roomPlayerID == "" {
			roomPlayerID = roomResp.RoomPlayer.ID
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade error: %v", err)
		}

		client := &Client{
			hub:          hub,
			conn:         conn,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       roomID,
			GameID:       gameID,
			RoomPlayerID: roomPlayerID,
			ctx:          r.Context(),
		}

		hub.register <- client

		// Start goroutine to receive events
		go func() {
			for {
				var event store.GameEvent
				if err := conn.ReadJSON(&event); err != nil {
					return
				}
				mu.Lock()
				receivedEvents[roomPlayerID] = append(receivedEvents[roomPlayerID], &event)
				mu.Unlock()
			}
		}()

		go client.writePump()
		go client.readPump()
	}))
	defer server.Close()

	// Connect first client
	wsURL1 := "ws" + server.URL[4:] + "?room_player_id=" + roomResp.RoomPlayer.ID
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL1, nil)
	if err != nil {
		t.Fatalf("websocket dial error: %v", err)
	}
	defer conn1.Close()

	// Connect second client
	wsURL2 := "ws" + server.URL[4:] + "?room_player_id=" + joinResp.RoomPlayer.ID
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL2, nil)
	if err != nil {
		t.Fatalf("websocket dial error: %v", err)
	}
	defer conn2.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify both clients are registered
	count := hub.GetRoomClientCount(roomResp.Room.ID)
	if count != 2 {
		t.Errorf("expected 2 clients in room, got %d", count)
	}

	// Send event from first client
	eventReq := store.CreateGameEventRequest{
		GameID: gameResp.Game.ID,
		Type:   "broadcast_test",
		Payload: map[string]interface{}{
			"message": "broadcast message",
		},
	}

	eventJSON, err := json.Marshal(eventReq)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := conn1.WriteMessage(websocket.TextMessage, eventJSON); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// Give time for event processing and broadcasting
	time.Sleep(200 * time.Millisecond)

	// Verify that player2 received the event, but player1 (sender) did not
	mu.Lock()
	player1Events := receivedEvents[roomResp.RoomPlayer.ID]
	player2Events := receivedEvents[joinResp.RoomPlayer.ID]
	mu.Unlock()

	// Sender should not receive their own event
	if len(player1Events) > 0 {
		t.Error("player 1 (sender) should not receive their own broadcast event")
	}

	// Other players should receive the event
	if len(player2Events) == 0 {
		t.Error("player 2 did not receive broadcast event")
	} else {
		if player2Events[0].Type != "broadcast_test" {
			t.Errorf("player 2: expected event type 'broadcast_test', got %s", player2Events[0].Type)
		}
	}

	// Verify event was stored in database
	gameUUID, err := stringToUUID(gameResp.Game.ID)
	if err != nil {
		t.Fatalf("failed to convert game ID: %v", err)
	}

	events, err := queries.GetGameEventsByGameId(ctx, gameUUID)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Error("expected event in database")
	}
}
