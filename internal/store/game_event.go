package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vntrieu/avalon/internal/db"
)

// GameEvent represents a game event.
type GameEvent struct {
	ID           string                 `json:"id"`
	GameID       string                 `json:"game_id"`
	RoomPlayerID *string                `json:"room_player_id,omitempty"`
	Type         string                 `json:"type"`
	Payload      map[string]interface{} `json:"payload"`
	CreatedAt    time.Time              `json:"created_at"`
}

// CreateGameEventRequest contains the data needed to create a game event.
type CreateGameEventRequest struct {
	GameID       string                 `json:"game_id"`
	RoomPlayerID *string                `json:"room_player_id,omitempty"`
	Type         string                 `json:"type"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

// GameEventStore handles database operations for game events.
type GameEventStore struct {
	pool    *db.Queries
	queries *db.Queries
}

// NewGameEventStore creates a new GameEventStore.
func NewGameEventStore(queries *db.Queries) *GameEventStore {
	return &GameEventStore{
		pool:    queries,
		queries: queries,
	}
}

// CreateGameEvent creates a new game event.
func (s *GameEventStore) CreateGameEvent(ctx context.Context, req CreateGameEventRequest) (*GameEvent, error) {
	// Convert game_id to UUID
	gameUUID, err := stringToUUID(req.GameID)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id: %w", err)
	}

	// Convert room_player_id to UUID if provided
	var roomPlayerUUID pgtype.UUID
	if req.RoomPlayerID != nil && *req.RoomPlayerID != "" {
		uuid, err := stringToUUID(*req.RoomPlayerID)
		if err != nil {
			return nil, fmt.Errorf("invalid room_player_id: %w", err)
		}
		roomPlayerUUID = uuid
	}

	// Serialize payload to JSONB
	payloadJSON := []byte("{}")
	if req.Payload != nil && len(req.Payload) > 0 {
		var err error
		payloadJSON, err = json.Marshal(req.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
	}

	// Create event
	createParams := db.CreateGameEventParams{
		GameID:       gameUUID,
		RoomPlayerID: roomPlayerUUID,
		Type:         req.Type,
		PayloadJson:  payloadJSON,
	}

	eventRow, err := s.queries.CreateGameEvent(ctx, createParams)
	if err != nil {
		return nil, fmt.Errorf("create game event: %w", err)
	}

	// Parse payload back
	var payload map[string]interface{}
	if err := json.Unmarshal(eventRow.PayloadJson, &payload); err != nil {
		payload = make(map[string]interface{})
	}

	var roomPlayerID *string
	if eventRow.RoomPlayerID.Valid {
		id := uuidToString(eventRow.RoomPlayerID)
		roomPlayerID = &id
	}

	event := &GameEvent{
		ID:           uuidToString(eventRow.ID),
		GameID:       uuidToString(eventRow.GameID),
		RoomPlayerID: roomPlayerID,
		Type:         eventRow.Type,
		Payload:      payload,
		CreatedAt:    timestamptzToTime(eventRow.CreatedAt),
	}

	return event, nil
}

// GetGameEvents retrieves all events for a game.
func (s *GameEventStore) GetGameEvents(ctx context.Context, gameID string) ([]GameEvent, error) {
	gameUUID, err := stringToUUID(gameID)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id: %w", err)
	}

	eventRows, err := s.queries.GetGameEventsByGameId(ctx, gameUUID)
	if err != nil {
		return nil, fmt.Errorf("get game events: %w", err)
	}

	events := make([]GameEvent, 0, len(eventRows))
	for _, eventRow := range eventRows {
		var payload map[string]interface{}
		if err := json.Unmarshal(eventRow.PayloadJson, &payload); err != nil {
			payload = make(map[string]interface{})
		}

		var roomPlayerID *string
		if eventRow.RoomPlayerID.Valid {
			id := uuidToString(eventRow.RoomPlayerID)
			roomPlayerID = &id
		}

		event := GameEvent{
			ID:           uuidToString(eventRow.ID),
			GameID:       uuidToString(eventRow.GameID),
			RoomPlayerID: roomPlayerID,
			Type:         eventRow.Type,
			Payload:      payload,
			CreatedAt:    timestamptzToTime(eventRow.CreatedAt),
		}
		events = append(events, event)
	}

	return events, nil
}
