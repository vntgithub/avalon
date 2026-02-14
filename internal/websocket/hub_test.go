package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/vntrieu/avalon/internal/store"
)

func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create a mock client
	client := &Client{
		hub:          hub,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       "room-1",
		GameID:       "game-1",
		RoomPlayerID: "player-1",
		ctx:          context.Background(),
	}

	// Register client
	hub.register <- client

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify client is registered
	count := hub.GetRoomClientCount("room-1")
	if count != 1 {
		t.Errorf("expected 1 client in room, got %d", count)
	}

	// Unregister client
	hub.unregister <- client

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify client is unregistered
	count = hub.GetRoomClientCount("room-1")
	if count != 0 {
		t.Errorf("expected 0 clients in room after unregister, got %d", count)
	}
}

func TestHub_MultipleClientsSameRoom(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create multiple clients in the same room
	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{
			hub:          hub,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       "room-1",
			GameID:       "game-1",
			RoomPlayerID: "player-" + string(rune('1'+i)),
			ctx:          context.Background(),
		}
		hub.register <- clients[i]
	}

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify all clients are registered
	count := hub.GetRoomClientCount("room-1")
	if count != 3 {
		t.Errorf("expected 3 clients in room, got %d", count)
	}

	// Unregister one client
	hub.unregister <- clients[0]

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify remaining clients
	count = hub.GetRoomClientCount("room-1")
	if count != 2 {
		t.Errorf("expected 2 clients in room after unregister, got %d", count)
	}
}

func TestHub_MultipleRooms(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create clients in different rooms
	room1Clients := make([]*Client, 2)
	room2Clients := make([]*Client, 2)

	for i := 0; i < 2; i++ {
		room1Clients[i] = &Client{
			hub:          hub,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       "room-1",
			GameID:       "game-1",
			RoomPlayerID: "player-" + string(rune('1'+i)),
			ctx:          context.Background(),
		}
		hub.register <- room1Clients[i]

		room2Clients[i] = &Client{
			hub:          hub,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       "room-2",
			GameID:       "game-2",
			RoomPlayerID: "player-" + string(rune('1'+i)),
			ctx:          context.Background(),
		}
		hub.register <- room2Clients[i]
	}

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify both rooms have correct client counts
	count1 := hub.GetRoomClientCount("room-1")
	if count1 != 2 {
		t.Errorf("expected 2 clients in room-1, got %d", count1)
	}

	count2 := hub.GetRoomClientCount("room-2")
	if count2 != 2 {
		t.Errorf("expected 2 clients in room-2, got %d", count2)
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create multiple clients in the same room
	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{
			hub:          hub,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       "room-1",
			GameID:       "game-1",
			RoomPlayerID: "player-" + string(rune('1'+i)),
			ctx:          context.Background(),
		}
		hub.register <- clients[i]
	}

	// Give hub time to process registration
	time.Sleep(10 * time.Millisecond)

	// Create a test event
	event := &store.GameEvent{
		ID:        "event-1",
		GameID:    "game-1",
		Type:      "test_event",
		Payload:   map[string]interface{}{"message": "test"},
		CreatedAt: time.Now(),
	}

	// Broadcast event
	hub.Broadcast("room-1", event)

	// Give hub time to process broadcast
	time.Sleep(10 * time.Millisecond)

	// Verify all clients received the event
	for i, client := range clients {
		select {
		case out := <-client.send:
			if out.GameEvent == nil {
				t.Errorf("client %d: expected GameEvent", i)
				continue
			}
			receivedEvent := out.GameEvent
			if receivedEvent.ID != event.ID {
				t.Errorf("client %d: expected event ID %s, got %s", i, event.ID, receivedEvent.ID)
			}
			if receivedEvent.Type != event.Type {
				t.Errorf("client %d: expected event type %s, got %s", i, event.Type, receivedEvent.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("client %d: did not receive broadcast event", i)
		}
	}
}

func TestHub_BroadcastToSpecificRoom(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Create clients in different rooms
	room1Client := &Client{
		hub:          hub,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       "room-1",
		GameID:       "game-1",
		RoomPlayerID: "player-1",
		ctx:          context.Background(),
	}
	hub.register <- room1Client

	room2Client := &Client{
		hub:          hub,
		send:         make(chan *OutgoingMessage, 256),
		RoomID:       "room-2",
		GameID:       "game-2",
		RoomPlayerID: "player-1",
		ctx:          context.Background(),
	}
	hub.register <- room2Client

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Broadcast event to room-1 only
	event := &store.GameEvent{
		ID:        "event-1",
		GameID:    "game-1",
		Type:      "test_event",
		Payload:   map[string]interface{}{"message": "test"},
		CreatedAt: time.Now(),
	}

	hub.Broadcast("room-1", event)

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Verify room-1 client received the event
	select {
	case out := <-room1Client.send:
		if out.GameEvent == nil {
			t.Error("room-1 client: expected GameEvent")
		} else if out.GameEvent.ID != event.ID {
			t.Errorf("room-1 client: expected event ID %s, got %s", event.ID, out.GameEvent.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("room-1 client: did not receive broadcast event")
	}

	// Verify room-2 client did NOT receive the event
	select {
	case <-room2Client.send:
		t.Error("room-2 client: should not have received event from room-1")
	case <-time.After(50 * time.Millisecond):
		// Expected - room-2 client should not receive the event
	}
}

func TestHub_EmptyRoomBroadcast(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Broadcast to a room with no clients (should not panic)
	event := &store.GameEvent{
		ID:        "event-1",
		GameID:    "game-1",
		Type:      "test_event",
		Payload:   map[string]interface{}{"message": "test"},
		CreatedAt: time.Now(),
	}

	hub.Broadcast("non-existent-room", event)

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	// Should not panic or error
	count := hub.GetRoomClientCount("non-existent-room")
	if count != 0 {
		t.Errorf("expected 0 clients in non-existent room, got %d", count)
	}
}

func TestHub_ConcurrentRegistration(t *testing.T) {
	hub := NewHub(nil)
	go hub.Run()

	// Register multiple clients concurrently
	clients := make([]*Client, 10)
	for i := 0; i < 10; i++ {
		clients[i] = &Client{
			hub:          hub,
			send:         make(chan *OutgoingMessage, 256),
			RoomID:       "room-1",
			GameID:       "game-1",
			RoomPlayerID: "player-" + string(rune('1'+i)),
			ctx:          context.Background(),
		}
		go func(c *Client) {
			hub.register <- c
		}(clients[i])
	}

	// Give hub time to process all registrations
	time.Sleep(50 * time.Millisecond)

	// Verify all clients are registered
	count := hub.GetRoomClientCount("room-1")
	if count != 10 {
		t.Errorf("expected 10 clients in room, got %d", count)
	}
}
