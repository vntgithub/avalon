package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vntrieu/avalon/internal/store"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, you should check the origin
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages (game events or server envelopes)
	send chan *OutgoingMessage

	// Room ID this client belongs to
	RoomID string

	// Game ID this client is connected to (empty for room-only WS)
	GameID string

	// Room Player ID (for identifying the sender)
	RoomPlayerID string

	// DisplayName for chat and broadcasts (room WS)
	DisplayName string

	// RateLimitKey is set at connection time (e.g. client IP) for rate limiting chat/actions.
	RateLimitKey string

	// Request context
	ctx context.Context
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		if c.GameID != "" {
			// Game WS: parse as game event request
			var eventReq store.CreateGameEventRequest
			if err := json.Unmarshal(message, &eventReq); err != nil {
				log.Printf("error unmarshaling game message: %v", err)
				continue
			}
			eventReq.GameID = c.GameID
			if c.RoomPlayerID != "" {
				eventReq.RoomPlayerID = &c.RoomPlayerID
			}
			if c.hub.eventHandler != nil {
				c.hub.eventHandler.HandleEvent(c.ctx, c, &eventReq)
			}
			continue
		}

		// Room WS: parse as client envelope and dispatch by type
		var clientMsg ClientInMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			log.Printf("error unmarshaling room message: %v", err)
			continue
		}
		if c.hub.eventHandler != nil {
			c.hub.eventHandler.HandleRoomMessage(c.ctx, c, &clientMsg)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case out, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			var payload interface{}
			if out.GameEvent != nil {
				payload = out.GameEvent
			} else {
				payload = out.Envelope
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				log.Printf("error encoding outbound message: %v", err)
			}

			// Drain queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				next := <-c.send
				if next.GameEvent != nil {
					payload = next.GameEvent
				} else {
					payload = next.Envelope
				}
				if err := json.NewEncoder(w).Encode(payload); err != nil {
					log.Printf("error encoding queued message: %v", err)
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

