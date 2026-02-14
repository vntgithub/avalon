package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vntrieu/avalon/internal/db"
)

// Game represents a game instance.
type Game struct {
	ID        string                 `json:"id"`
	RoomID    string                 `json:"room_id"`
	Status    string                 `json:"status"` // waiting | in_progress | finished
	Config    map[string]interface{} `json:"config"`
	CreatedAt time.Time              `json:"created_at"`
	EndedAt   *time.Time             `json:"ended_at,omitempty"`
}

// GamePlayer represents a player in a game.
type GamePlayer struct {
	ID           string     `json:"id"`
	GameID       string     `json:"game_id"`
	RoomPlayerID string     `json:"room_player_id"`
	Role         *string    `json:"role,omitempty"`
	JoinedAt     time.Time  `json:"joined_at"`
	LeftAt       *time.Time `json:"left_at,omitempty"`
}

// CreateGameRequest contains the data needed to create a game.
// Exactly one of Code or RoomID must be set. Code is the room's join code; RoomID is the room UUID.
type CreateGameRequest struct {
	Code   string                 `json:"code,omitempty"`   // room join code (preferred)
	RoomID string                 `json:"room_id,omitempty"` // room UUID (e.g. for internal use)
	Config map[string]interface{} `json:"config,omitempty"`
}

// CreateGameResponse contains the response after creating a game.
type CreateGameResponse struct {
	Game                    *Game                  `json:"game"`
	Players                 []GamePlayer           `json:"players"`
	LatestGameStateSnapshot map[string]interface{} `json:"latest_game_state_snapshot,omitempty"`
}

// LobbyStateJSON is the initial snapshot state for a new game (phase: lobby).
var LobbyStateJSON = []byte(`{"phase":"lobby"}`)

// GameStore handles database operations for games.
type GameStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewGameStore creates a new GameStore.
func NewGameStore(pool *pgxpool.Pool) *GameStore {
	return &GameStore{
		pool:    pool,
		queries: db.New(pool),
	}
}

// CreateGame creates a new game in a room with all room players.
func (s *GameStore) CreateGame(ctx context.Context, req CreateGameRequest) (*CreateGameResponse, error) {
	var roomUUID pgtype.UUID
	if req.Code != "" {
		roomRow, err := s.queries.GetRoomByCode(ctx, req.Code)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("room not found")
			}
			return nil, fmt.Errorf("get room by code: %w", err)
		}
		roomUUID = roomRow.ID
	} else if req.RoomID != "" {
		var err error
		roomUUID, err = stringToUUID(req.RoomID)
		if err != nil {
			return nil, fmt.Errorf("invalid room_id: %w", err)
		}
		_, err = s.queries.GetRoomById(ctx, roomUUID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("room not found")
			}
			return nil, fmt.Errorf("get room: %w", err)
		}
	} else {
		return nil, fmt.Errorf("code or room_id is required")
	}

	// Get all room players
	roomPlayers, err := s.queries.GetRoomPlayersByRoomId(ctx, roomUUID)
	if err != nil {
		return nil, fmt.Errorf("get room players: %w", err)
	}

	if len(roomPlayers) == 0 {
		return nil, fmt.Errorf("cannot create game: room has no players")
	}

	// Serialize config to JSONB
	configJSON := []byte("{}")
	if len(req.Config) > 0 {
		var err error
		configJSON, err = json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("marshal config: %w", err)
		}
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	// Create game with status "waiting"
	createGameParams := db.CreateGameParams{
		RoomID:     roomUUID,
		Status:     "waiting",
		ConfigJson: configJSON,
	}
	gameRow, err := txQueries.CreateGame(ctx, createGameParams)
	if err != nil {
		return nil, fmt.Errorf("create game: %w", err)
	}

	gameID := uuidToString(gameRow.ID)
	gameUUID := gameRow.ID

	// Add all room players to the game
	gamePlayers := make([]GamePlayer, 0, len(roomPlayers))
	for _, roomPlayer := range roomPlayers {
		createPlayerParams := db.CreateGamePlayerParams{
			GameID:       gameUUID,
			RoomPlayerID: roomPlayer.ID,
			Role:         pgtype.Text{Valid: false}, // Role assigned later during game setup
		}
		gamePlayerRow, err := txQueries.CreateGamePlayer(ctx, createPlayerParams)
		if err != nil {
			return nil, fmt.Errorf("create game player: %w", err)
		}

		var role *string
		if gamePlayerRow.Role.Valid {
			role = &gamePlayerRow.Role.String
		}

		var leftAt *time.Time
		if gamePlayerRow.LeftAt.Valid {
			t := timestamptzToTime(gamePlayerRow.LeftAt)
			leftAt = &t
		}

		gamePlayer := GamePlayer{
			ID:           uuidToString(gamePlayerRow.ID),
			GameID:       gameID,
			RoomPlayerID: uuidToString(gamePlayerRow.RoomPlayerID),
			Role:         role,
			JoinedAt:     timestamptzToTime(gamePlayerRow.JoinedAt),
			LeftAt:       leftAt,
		}
		gamePlayers = append(gamePlayers, gamePlayer)
	}

	// Create initial snapshot (version 1, lobby state)
	snapshotParams := db.CreateGameStateSnapshotParams{
		GameID:    gameUUID,
		Version:   1,
		StateJson: LobbyStateJSON,
	}
	snapshotRow, err := txQueries.CreateGameStateSnapshot(ctx, snapshotParams)
	if err != nil {
		return nil, fmt.Errorf("create initial snapshot: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Parse config back
	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		config = make(map[string]interface{})
	}

	var endedAt *time.Time
	if gameRow.EndedAt.Valid {
		t := timestamptzToTime(gameRow.EndedAt)
		endedAt = &t
	}

	game := &Game{
		ID:        gameID,
		RoomID:    uuidToString(gameRow.RoomID),
		Status:    gameRow.Status,
		Config:    config,
		CreatedAt: timestamptzToTime(gameRow.CreatedAt),
		EndedAt:   endedAt,
	}

	var snapshotMap map[string]interface{}
	if len(snapshotRow.StateJson) > 0 {
		_ = json.Unmarshal(snapshotRow.StateJson, &snapshotMap)
	}
	if snapshotMap == nil {
		snapshotMap = make(map[string]interface{})
	}

	return &CreateGameResponse{
		Game:                    game,
		Players:                 gamePlayers,
		LatestGameStateSnapshot: snapshotMap,
	}, nil
}

// GetLatestGameForRoom returns the most recently created game for the room (by created_at DESC).
func (s *GameStore) GetLatestGameForRoom(ctx context.Context, roomID string) (*Game, error) {
	roomUUID, err := stringToUUID(roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room_id: %w", err)
	}
	games, err := s.queries.GetGamesByRoomId(ctx, roomUUID)
	if err != nil {
		return nil, fmt.Errorf("get games by room: %w", err)
	}
	if len(games) == 0 {
		return nil, nil
	}
	return dbGameToStoreGame(&games[0]), nil
}

// CreateOrUpdateSnapshot creates a new snapshot for the game with the next version number.
// stateJSON is the full state to store. Returns the new snapshot's version.
func (s *GameStore) CreateOrUpdateSnapshot(ctx context.Context, gameID string, stateJSON map[string]interface{}) (version int32, err error) {
	gameUUID, err := stringToUUID(gameID)
	if err != nil {
		return 0, fmt.Errorf("invalid game_id: %w", err)
	}
	data := []byte("{}")
	if len(stateJSON) > 0 {
		data, err = json.Marshal(stateJSON)
		if err != nil {
			return 0, fmt.Errorf("marshal state: %w", err)
		}
	}
	var nextVersion int32 = 1
	snapshot, err := s.queries.GetLatestGameStateSnapshotByGameId(ctx, gameUUID)
	if err != nil && err != pgx.ErrNoRows {
		return 0, fmt.Errorf("get latest snapshot: %w", err)
	}
	if err == nil {
		nextVersion = snapshot.Version + 1
	}
	_, err = s.queries.CreateGameStateSnapshot(ctx, db.CreateGameStateSnapshotParams{
		GameID:    gameUUID,
		Version:   nextVersion,
		StateJson: data,
	})
	if err != nil {
		return 0, fmt.Errorf("create snapshot: %w", err)
	}
	return nextVersion, nil
}

// GetLatestSnapshot returns the latest game state snapshot as a map, or nil if none exists.
func (s *GameStore) GetLatestSnapshot(ctx context.Context, gameID string) (map[string]interface{}, error) {
	gameUUID, err := stringToUUID(gameID)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id: %w", err)
	}
	snapshot, err := s.queries.GetLatestGameStateSnapshotByGameId(ctx, gameUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}
	var out map[string]interface{}
	if len(snapshot.StateJson) > 0 {
		if err := json.Unmarshal(snapshot.StateJson, &out); err != nil {
			return nil, fmt.Errorf("unmarshal snapshot: %w", err)
		}
	}
	if out == nil {
		out = make(map[string]interface{})
	}
	return out, nil
}

// UpdateGameStatus updates the game's status and optionally ended_at.
func (s *GameStore) UpdateGameStatus(ctx context.Context, gameID string, status string, endedAt *time.Time) error {
	gameUUID, err := stringToUUID(gameID)
	if err != nil {
		return fmt.Errorf("invalid game_id: %w", err)
	}
	var endAt pgtype.Timestamptz
	if endedAt != nil {
		endAt = pgtype.Timestamptz{Time: *endedAt, Valid: true}
	}
	return s.queries.UpdateGameStatus(ctx, db.UpdateGameStatusParams{
		ID:      gameUUID,
		Status:  status,
		EndedAt: endAt,
	})
}

// GetGamePlayerIDsInOrder returns room_player_id list for the game in display order (by room join order).
func (s *GameStore) GetGamePlayerIDsInOrder(ctx context.Context, gameID string) ([]string, error) {
	gameUUID, err := stringToUUID(gameID)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id: %w", err)
	}
	players, err := s.queries.GetRoomPlayersByGameId(ctx, gameUUID)
	if err != nil {
		return nil, fmt.Errorf("get game players: %w", err)
	}
	ids := make([]string, 0, len(players))
	for _, p := range players {
		ids = append(ids, uuidToString(p.ID))
	}
	return ids, nil
}
