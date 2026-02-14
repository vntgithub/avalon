package websocket

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vntrieu/avalon/internal/db"
	"github.com/vntrieu/avalon/internal/games"
	"github.com/vntrieu/avalon/internal/ratelimit"
	"github.com/vntrieu/avalon/internal/store"
)

// EventHandler handles game events and broadcasts them.
type EventHandler struct {
	hub         *Hub
	eventStore  *store.GameEventStore
	gameStore   *store.GameStore
	engine      *games.Engine
	queries     *db.Queries
	rateLimiter ratelimit.Limiter
}

// NewGameEngine creates a game engine with the given game store and pool (for event store).
func NewGameEngine(gameStore *store.GameStore, pool *pgxpool.Pool) *games.Engine {
	queries := db.New(pool)
	eventStore := store.NewGameEventStore(queries)
	return games.NewEngine(gameStore, eventStore, games.ClassicAvalonConfig())
}

// NewEventHandler creates a new EventHandler. hub may be nil when building the hub. engine may be nil to create a default one.
// rateLimiter is optional; when set, chat messages are rate-limited by client key (e.g. IP).
func NewEventHandler(hub *Hub, pool *pgxpool.Pool, gameStore *store.GameStore, engine *games.Engine, rateLimiter ratelimit.Limiter) *EventHandler {
	queries := db.New(pool)
	eventStore := store.NewGameEventStore(queries)
	if engine == nil && gameStore != nil {
		engine = games.NewEngine(gameStore, eventStore, games.ClassicAvalonConfig())
	}
	return &EventHandler{
		hub:         hub,
		eventStore:  eventStore,
		gameStore:   gameStore,
		engine:      engine,
		queries:     queries,
		rateLimiter: rateLimiter,
	}
}

// HandleRoomMessage processes an incoming room message (chat, vote, action, sync_state).
// Rejects unknown or invalid message types with an error envelope.
func (h *EventHandler) HandleRoomMessage(ctx context.Context, client *Client, msg *ClientInMessage) {
	if msg == nil {
		sendErrorToClient(client, "invalid message")
		return
	}
	// Validate type: allowlist and length to prevent abuse
	if len(msg.Type) > MaxClientMessageTypeLength {
		sendErrorToClient(client, "invalid message type")
		return
	}
	if !ValidClientMessageTypes[msg.Type] {
		sendErrorToClient(client, "unsupported message type")
		return
	}
	switch msg.Type {
	case ClientMessageTypeChat:
		h.handleChat(ctx, client, msg)
	case ClientMessageTypeVote:
		h.handleVote(ctx, client, msg)
	case ClientMessageTypeAction:
		h.handleAction(ctx, client, msg)
	case ClientMessageTypeSyncState:
		h.handleSyncState(ctx, client, msg)
	default:
		sendErrorToClient(client, "unsupported message type")
	}
}

// handleSyncState loads the latest game snapshot for the client's room and sends a state message to that client only.
func (h *EventHandler) handleSyncState(ctx context.Context, client *Client, msg *ClientInMessage) {
	if h.gameStore == nil || h.engine == nil {
		sendErrorToClient(client, "sync_state not available")
		return
	}
	game, err := h.gameStore.GetLatestGameForRoom(ctx, client.RoomID)
	if err != nil || game == nil {
		sendErrorToClient(client, "no game found for room")
		return
	}
	state, err := h.engine.GetState(ctx, game.ID)
	if err != nil {
		sendErrorToClient(client, "failed to load state")
		return
	}
	payload := map[string]interface{}{"game_id": game.ID}
	if state != nil {
		payload["state"] = state.ToMap()
		payload["phase"] = state.Phase
		payload["version"] = state.Version
	} else {
		payload["state"] = map[string]interface{}{"phase": "lobby"}
	}
	envelope := &ServerEnvelope{Type: ServerTypeState, Event: ServerEventState, Payload: payload}
	sendEnvelopeToClient(client, envelope)
}

// handleVote parses payload and calls engine ApplyMove with type "vote"; broadcasts result or sends error to client.
func (h *EventHandler) handleVote(ctx context.Context, client *Client, msg *ClientInMessage) {
	if h.gameStore == nil || h.engine == nil {
		sendErrorToClient(client, "vote not available")
		return
	}
	game, err := h.gameStore.GetLatestGameForRoom(ctx, client.RoomID)
	if err != nil || game == nil {
		sendErrorToClient(client, "no game found for room")
		return
	}
	payload := games.DecodePayload(msg.Payload)
	if payload == nil {
		payload = make(map[string]interface{})
	}
	result := h.engine.ApplyMove(ctx, game.ID, client.RoomPlayerID, "vote", payload)
	if result.Error != nil {
		sendErrorToClient(client, result.Error.Error())
		return
	}
	h.broadcastResult(ctx, client, game.ID, result)
}

// handleAction parses payload (action type + params) and calls engine ApplyMove with type "action".
func (h *EventHandler) handleAction(ctx context.Context, client *Client, msg *ClientInMessage) {
	if h.gameStore == nil || h.engine == nil {
		sendErrorToClient(client, "action not available")
		return
	}
	game, err := h.gameStore.GetLatestGameForRoom(ctx, client.RoomID)
	if err != nil || game == nil {
		sendErrorToClient(client, "no game found for room")
		return
	}
	payload := games.DecodePayload(msg.Payload)
	if payload == nil {
		payload = make(map[string]interface{})
	}
	result := h.engine.ApplyMove(ctx, game.ID, client.RoomPlayerID, "action", payload)
	if result.Error != nil {
		sendErrorToClient(client, result.Error.Error())
		return
	}
	h.broadcastResult(ctx, client, game.ID, result)
}

// broadcastResult sends result.Events to the room and optionally a state envelope with the new state.
func (h *EventHandler) broadcastResult(ctx context.Context, client *Client, gameID string, result games.ApplyMoveResult) {
	if h.hub == nil {
		return
	}
	for _, ev := range result.Events {
		envelope := &ServerEnvelope{Type: ServerTypeEvent, Event: ev.Event, Payload: ev.Payload}
		h.hub.BroadcastEnvelope(client.RoomID, envelope)
	}
	if result.State != nil {
		statePayload := map[string]interface{}{
			"game_id": gameID,
			"state":   result.State.ToMap(),
			"phase":   result.State.Phase,
			"version": result.State.Version,
		}
		h.hub.BroadcastEnvelope(client.RoomID, &ServerEnvelope{Type: ServerTypeState, Event: ServerEventState, Payload: statePayload})
	}
}

func sendErrorToClient(client *Client, message string) {
	sendEnvelopeToClient(client, &ServerEnvelope{Type: ServerTypeError, Payload: map[string]interface{}{"message": message}})
}

func sendEnvelopeToClient(client *Client, envelope *ServerEnvelope) {
	select {
	case client.send <- &OutgoingMessage{Envelope: envelope}:
	default:
		log.Printf("could not send envelope to client (channel full)")
	}
}

// handleChat persists (optional) and broadcasts a chat message to the room.
func (h *EventHandler) handleChat(ctx context.Context, client *Client, msg *ClientInMessage) {
	if h.rateLimiter != nil && client.RateLimitKey != "" {
		allowed, _ := h.rateLimiter.Allow(client.RateLimitKey)
		if !allowed {
			sendErrorToClient(client, "rate limit exceeded; try again later")
			return
		}
	}
	var message string
	if msg.Payload != nil {
		if m, ok := msg.Payload["message"].(string); ok {
			message = m
		}
	}
	message = trimToMax(message, MaxChatMessageLength)
	if message == "" {
		return
	}
	roomUUID, err := stringToUUID(client.RoomID)
	if err != nil {
		return
	}
	playerUUID, err := stringToUUID(client.RoomPlayerID)
	if err != nil {
		return
	}
	// Optional: persist to chat_messages (room-level chat, no game_id)
	_, _ = h.queries.CreateChatMessage(ctx, db.CreateChatMessageParams{
		RoomID:       roomUUID,
		GameID:       pgtype.UUID{Valid: false},
		RoomPlayerID: playerUUID,
		Message:      message,
	})
	envelope := &ServerEnvelope{
		Type:  ServerTypeEvent,
		Event: ServerEventChat,
		Payload: map[string]interface{}{
			"display_name": client.DisplayName,
			"message":      message,
		},
	}
	h.hub.BroadcastEnvelopeExcept(client.RoomID, envelope, client)
}

func trimToMax(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// HandleEvent processes an incoming event from a client.
func (h *EventHandler) HandleEvent(ctx context.Context, client *Client, eventReq *store.CreateGameEventRequest) {
	// Validate game exists and get room_id
	gameUUID, err := stringToUUID(eventReq.GameID)
	if err != nil {
		log.Printf("game event: invalid game_id=%q: %v", eventReq.GameID, err)
		return
	}

	game, err := h.queries.GetGameById(ctx, gameUUID)
	if err != nil {
		log.Printf("game event: game_id=%s not found: %v", eventReq.GameID, err)
		return
	}

	roomID := uuidToString(game.RoomID)

	// Create event in database
	event, err := h.eventStore.CreateGameEvent(ctx, *eventReq)
	if err != nil {
		log.Printf("game event: game_id=%s room_id=%s error creating event: %v", eventReq.GameID, roomID, err)
		return
	}

	// Broadcast to all clients in the room except the sender (do not log payload; may contain sensitive data)
	h.hub.BroadcastExcept(roomID, event, client)
	log.Printf("broadcast game_id=%s room_id=%s event_id=%s type=%s", eventReq.GameID, roomID, event.ID, event.Type)
}

// Helper function to convert string to UUID
func stringToUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	var u pgtype.UUID
	copy(u.Bytes[:], id[:])
	u.Valid = true
	return u, nil
}

// Helper function to convert UUID to string
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}
