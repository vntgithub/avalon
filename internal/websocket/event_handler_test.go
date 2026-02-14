package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/vntrieu/avalon/internal/db"
	"github.com/vntrieu/avalon/internal/store"
)

func TestEventHandler_HandleEvent(t *testing.T) {
	pool := store.SetupTestDB(t)
	defer pool.Close()

	hub := NewHub(nil)
	gameStore := store.NewGameStore(pool)
	eventHandler := NewEventHandler(hub, pool, gameStore, nil, nil)
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

	createGameReq := store.CreateGameRequest{
		RoomID: roomResp.Room.ID,
	}
	gameResp, err := gameStore.CreateGame(ctx, createGameReq)
	if err != nil {
		t.Fatalf("failed to create game: %v", err)
	}

	// Create a mock client
	client := &Client{
		hub:          hub,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       roomResp.Room.ID,
		GameID:       gameResp.Game.ID,
		RoomPlayerID: roomResp.RoomPlayer.ID,
		ctx:          ctx,
	}

	// Register client
	hub.register <- client

	// Give time for registration
	time.Sleep(10 * time.Millisecond)

	// Create event request
	eventReq := &store.CreateGameEventRequest{
		GameID: gameResp.Game.ID,
		Type:   "test_event",
		Payload: map[string]interface{}{
			"test": "data",
		},
	}

	// Handle event
	eventHandler.HandleEvent(ctx, client, eventReq)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Verify event was NOT broadcast back to sender (sender should not receive their own event)
	select {
	case out := <-client.send:
		if out.GameEvent != nil {
			t.Errorf("sender should not receive their own event, but received: %s", out.GameEvent.Type)
		}
	case <-time.After(100 * time.Millisecond):
		// Expected - sender should not receive their own event
	}

	// Verify event was created in database
	queries := db.New(pool)
	eventStore := store.NewGameEventStore(queries)
	events, err := eventStore.GetGameEvents(ctx, gameResp.Game.ID)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) == 0 {
		t.Error("event should have been created in database")
	} else {
		if events[0].Type != "test_event" {
			t.Errorf("expected event type 'test_event', got %s", events[0].Type)
		}
	}
}

func TestEventHandler_InvalidGameID(t *testing.T) {
	pool := store.SetupTestDB(t)
	defer pool.Close()

	hub := NewHub(nil)
	gameStore := store.NewGameStore(pool)
	eventHandler := NewEventHandler(hub, pool, gameStore, nil, nil)
	hub.SetEventHandler(eventHandler)
	go hub.Run()

	client := &Client{
		hub:          hub,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       "room-1",
		GameID:       "invalid-game-id",
		RoomPlayerID: "player-1",
		ctx:          context.Background(),
	}

	eventReq := &store.CreateGameEventRequest{
		GameID: "invalid-game-id",
		Type:   "test_event",
	}

	// Should not panic, but event won't be created
	eventHandler.HandleEvent(context.Background(), client, eventReq)

	// Give time for processing
	time.Sleep(10 * time.Millisecond)

	// Client should not receive event (game doesn't exist)
	select {
	case <-client.send:
		t.Error("client should not receive event for invalid game")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event should be sent
	}
}
