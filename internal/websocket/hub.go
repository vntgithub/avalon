package websocket

import (
	"log"
	"sync"

	"github.com/vntrieu/avalon/internal/store"
)

// Hub maintains the set of active clients and broadcasts messages to clients.
type Hub struct {
	// Registered clients by room_id -> client map
	rooms map[string]map[*Client]bool

	// Inbound messages from the clients
	broadcast chan *BroadcastMessage

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Event handler for processing events
	eventHandler *EventHandler

	// Mutex for thread-safe access
	mu sync.RWMutex
}

// BroadcastMessage represents a message to be broadcast to a room.
// Exactly one of Event or Envelope should be set.
type BroadcastMessage struct {
	RoomID        string
	Event         *store.GameEvent  // for game WS
	Envelope      *ServerEnvelope   // for room WS (e.g. chat)
	ExcludeClient *Client           // Optional: exclude this client from the broadcast
}

// NewHub creates a new Hub.
func NewHub(eventHandler *EventHandler) *Hub {
	return &Hub{
		rooms:        make(map[string]map[*Client]bool),
		broadcast:    make(chan *BroadcastMessage, 256),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		eventHandler: eventHandler,
	}
}

// SetEventHandler sets the event handler for the hub.
func (h *Hub) SetEventHandler(handler *EventHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.eventHandler = handler
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.RoomID] == nil {
				h.rooms[client.RoomID] = make(map[*Client]bool)
			}
			h.rooms[client.RoomID][client] = true
			h.mu.Unlock()
			log.Printf("ws client registered room_id=%s player_id=%s total=%d", client.RoomID, client.RoomPlayerID, len(h.rooms[client.RoomID]))

		case client := <-h.unregister:
			h.mu.Lock()
			if room, ok := h.rooms[client.RoomID]; ok {
				if _, ok := room[client]; ok {
					delete(room, client)
					close(client.send)
					if len(room) == 0 {
						delete(h.rooms, client.RoomID)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("ws client unregistered room_id=%s player_id=%s", client.RoomID, client.RoomPlayerID)

		case message := <-h.broadcast:
			h.mu.RLock()
			room, exists := h.rooms[message.RoomID]
			if exists {
				var out *OutgoingMessage
				if message.Event != nil {
					out = &OutgoingMessage{GameEvent: message.Event}
				} else if message.Envelope != nil {
					out = &OutgoingMessage{Envelope: message.Envelope}
				}
				for client := range room {
					if out == nil {
						continue
					}
					if message.ExcludeClient != nil && client == message.ExcludeClient {
						continue
					}
					select {
					case client.send <- out:
					default:
						close(client.send)
						delete(room, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all clients in a room.
func (h *Hub) Broadcast(roomID string, event *store.GameEvent) {
	h.broadcast <- &BroadcastMessage{
		RoomID: roomID,
		Event:  event,
	}
}

// BroadcastExcept sends a message to all clients in a room except the specified client.
func (h *Hub) BroadcastExcept(roomID string, event *store.GameEvent, excludeClient *Client) {
	h.broadcast <- &BroadcastMessage{
		RoomID:        roomID,
		Event:         event,
		ExcludeClient: excludeClient,
	}
}

// BroadcastEnvelope sends a server envelope to all clients in a room (e.g. chat).
func (h *Hub) BroadcastEnvelope(roomID string, envelope *ServerEnvelope) {
	h.broadcast <- &BroadcastMessage{RoomID: roomID, Envelope: envelope}
}

// BroadcastEnvelopeExcept sends a server envelope to all clients in a room except the specified client.
func (h *Hub) BroadcastEnvelopeExcept(roomID string, envelope *ServerEnvelope, excludeClient *Client) {
	h.broadcast <- &BroadcastMessage{
		RoomID:        roomID,
		Envelope:      envelope,
		ExcludeClient: excludeClient,
	}
}

// GetRoomClientCount returns the number of clients in a room.
func (h *Hub) GetRoomClientCount(roomID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if room, ok := h.rooms[roomID]; ok {
		return len(room)
	}
	return 0
}
