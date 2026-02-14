package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/vntrieu/avalon/internal/db"
)

// Room represents a game room.
type Room struct {
	ID           string                 `json:"id"`
	Code         string                 `json:"code"`
	PasswordHash *string                `json:"-"` // Never expose password hash
	Settings     map[string]interface{} `json:"settings"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// RoomPlayer represents a player in a room.
type RoomPlayer struct {
	ID          string    `json:"id"`
	RoomID      string    `json:"room_id"`
	DisplayName string    `json:"display_name"`
	IsHost      bool      `json:"is_host"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRoomRequest contains the data needed to create a room.
type CreateRoomRequest struct {
	Password    string                 `json:"password,omitempty"`
	DisplayName string                 `json:"display_name"`
	Settings    map[string]interface{} `json:"settings,omitempty"`
}

// CreateRoomResponse contains the response after creating a room.
// Token and ExpiresAt are set by the HTTP handler after calling CreateRoom.
type CreateRoomResponse struct {
	Room       *Room       `json:"room"`
	RoomPlayer *RoomPlayer `json:"room_player"`
	Token      string     `json:"token,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// JoinRoomRequest contains the data needed to join a room.
type JoinRoomRequest struct {
	Code        string `json:"code"`
	Password    string `json:"password,omitempty"`
	DisplayName string `json:"display_name"`
}

// JoinRoomResponse contains the response after joining a room.
// Includes latest game and its latest state snapshot when the room has at least one game.
// Token and ExpiresAt are set by the HTTP handler after calling JoinRoom.
type JoinRoomResponse struct {
	Room                    *Room                   `json:"room"`
	RoomPlayer              *RoomPlayer             `json:"room_player"`
	LatestGame              *Game                   `json:"latest_game,omitempty"`
	GamePlayer              *GamePlayer             `json:"game_player,omitempty"` // New player's entry in latest game
	LatestGameStateSnapshot map[string]interface{}  `json:"latest_game_state_snapshot,omitempty"`
	Token                   string                 `json:"token,omitempty"`
	ExpiresAt               *time.Time             `json:"expires_at,omitempty"`
}

// GetRoomResponse contains room info, latest game descriptor, and latest snapshot for GET /api/rooms/{code}.
type GetRoomResponse struct {
	Room                    *Room                  `json:"room"`
	LatestGame              *Game                  `json:"latest_game,omitempty"`
	LatestGameStateSnapshot map[string]interface{} `json:"latest_game_state_snapshot,omitempty"`
}

// RoomStore handles database operations for rooms.
type RoomStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewRoomStore creates a new RoomStore.
func NewRoomStore(pool *pgxpool.Pool) *RoomStore {
	return &RoomStore{
		pool:    pool,
		queries: db.New(pool),
	}
}

// generateRoomCode generates a unique, human-readable room code.
func generateRoomCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Exclude confusing chars like 0, O, I, 1
	const codeLength = 6
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	code := make([]byte, codeLength)
	for i := range code {
		code[i] = charset[r.Intn(len(charset))]
	}
	return string(code)
}

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// uuidToString converts pgtype.UUID to string.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	// Convert [16]byte to uuid.UUID then to string
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}

// stringToUUID converts string to pgtype.UUID.
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

// textToString converts pgtype.Text to *string (nullable).
func textToString(text pgtype.Text) *string {
	if !text.Valid {
		return nil
	}
	return &text.String
}

// stringToText converts *string to pgtype.Text (nullable).
func stringToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// timestamptzToTime converts pgtype.Timestamptz to time.Time.
func timestamptzToTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

// CreateRoom creates a new room with the given settings and an initial host player.
func (s *RoomStore) CreateRoom(ctx context.Context, req CreateRoomRequest) (*CreateRoomResponse, error) {
	// Generate unique room code
	var code string
	for {
		code = generateRoomCode()
		exists, err := s.queries.CheckRoomCodeExists(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("check room code exists: %w", err)
		}
		if !exists {
			break
		}
	}

	// Hash password if provided
	var passwordHash *string
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			return nil, err
		}
		passwordHash = &hash
	}

	// Serialize settings to JSONB
	settingsJSON := []byte("{}")
	if len(req.Settings) > 0 {
		var err error
		settingsJSON, err = json.Marshal(req.Settings)
		if err != nil {
			return nil, fmt.Errorf("marshal settings: %w", err)
		}
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	// Insert room
	createRoomParams := db.CreateRoomParams{
		Code:         code,
		PasswordHash: stringToText(passwordHash),
		SettingsJson: settingsJSON,
	}
	createRoomRow, err := txQueries.CreateRoom(ctx, createRoomParams)
	if err != nil {
		return nil, fmt.Errorf("insert room: %w", err)
	}

	roomID := uuidToString(createRoomRow.ID)

	// Insert room player (host)
	roomUUID, err := stringToUUID(roomID)
	if err != nil {
		return nil, fmt.Errorf("convert room id to uuid: %w", err)
	}

	createPlayerParams := db.CreateRoomPlayerParams{
		RoomID:      roomUUID,
		DisplayName: req.DisplayName,
		IsHost:      true,
	}
	roomPlayerRow, err := txQueries.CreateRoomPlayer(ctx, createPlayerParams)
	if err != nil {
		return nil, fmt.Errorf("insert room player: %w", err)
	}

	// Create initial game (status waiting) and add host as game player
	createGameParams := db.CreateGameParams{
		RoomID:     roomUUID,
		Status:     "waiting",
		ConfigJson: []byte("{}"),
	}
	_, err = txQueries.CreateGame(ctx, createGameParams)
	if err != nil {
		return nil, fmt.Errorf("create initial game: %w", err)
	}
	games, err := txQueries.GetGamesByRoomId(ctx, roomUUID)
	if err != nil || len(games) == 0 {
		return nil, fmt.Errorf("get initial game: %w", err)
	}
	latestGame := games[0]
	createGamePlayerParams := db.CreateGamePlayerParams{
		GameID:       latestGame.ID,
		RoomPlayerID: roomPlayerRow.ID,
		Role:         pgtype.Text{Valid: false},
	}
	_, err = txQueries.CreateGamePlayer(ctx, createGamePlayerParams)
	if err != nil {
		return nil, fmt.Errorf("create game player for host: %w", err)
	}

	// Create initial snapshot (version 1, lobby state) for the initial game
	_, err = txQueries.CreateGameStateSnapshot(ctx, db.CreateGameStateSnapshotParams{
		GameID:    latestGame.ID,
		Version:   1,
		StateJson: LobbyStateJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("create initial game snapshot: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Parse settings back
	var settings map[string]interface{}
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		settings = make(map[string]interface{})
	}

	room := &Room{
		ID:        roomID,
		Code:      code,
		Settings:  settings,
		CreatedAt: timestamptzToTime(createRoomRow.CreatedAt),
		UpdatedAt: timestamptzToTime(createRoomRow.UpdatedAt),
	}

	roomPlayer := &RoomPlayer{
		ID:          uuidToString(roomPlayerRow.ID),
		RoomID:      roomID,
		DisplayName: roomPlayerRow.DisplayName,
		IsHost:      roomPlayerRow.IsHost,
		CreatedAt:   timestamptzToTime(roomPlayerRow.CreatedAt),
	}

	return &CreateRoomResponse{
		Room:       room,
		RoomPlayer: roomPlayer,
	}, nil
}

// JoinRoom allows a player to join an existing room by code.
func (s *RoomStore) JoinRoom(ctx context.Context, req JoinRoomRequest) (*JoinRoomResponse, error) {
	// Validate display name
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name is required")
	}

	// Find room by code
	roomRow, err := s.queries.GetRoomByCode(ctx, req.Code)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("room not found")
		}
		return nil, fmt.Errorf("get room by code: %w", err)
	}

	roomID := uuidToString(roomRow.ID)

	// Validate password if room has one
	passwordHash := textToString(roomRow.PasswordHash)
	if passwordHash != nil {
		if req.Password == "" {
			return nil, fmt.Errorf("password is required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(req.Password)); err != nil {
			return nil, fmt.Errorf("invalid password")
		}
	}

	// Check if display name already exists in room
	roomUUID, err := stringToUUID(roomID)
	if err != nil {
		return nil, fmt.Errorf("convert room id to uuid: %w", err)
	}

	checkParams := db.CheckDisplayNameExistsParams{
		RoomID:      roomUUID,
		DisplayName: req.DisplayName,
	}
	exists, err := s.queries.CheckDisplayNameExists(ctx, checkParams)
	if err != nil {
		return nil, fmt.Errorf("check display name exists: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("display name already taken in this room")
	}

	// Parse settings JSON
	var settings map[string]interface{}
	if err := json.Unmarshal(roomRow.SettingsJson, &settings); err != nil {
		settings = make(map[string]interface{})
	}

	// Insert room player and add to latest game in a transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	txQueries := s.queries.WithTx(tx)

	createPlayerParams := db.CreateRoomPlayerParams{
		RoomID:      roomUUID,
		DisplayName: req.DisplayName,
		IsHost:      false,
	}
	roomPlayerRow, err := txQueries.CreateRoomPlayer(ctx, createPlayerParams)
	if err != nil {
		return nil, fmt.Errorf("insert room player: %w", err)
	}

	var latestGame *Game
	var gamePlayer *GamePlayer
	games, err := txQueries.GetGamesByRoomId(ctx, roomUUID)
	if err != nil {
		return nil, fmt.Errorf("get games by room: %w", err)
	}
	if len(games) > 0 {
		latestGameRow := games[0]
		createGamePlayerParams := db.CreateGamePlayerParams{
			GameID:       latestGameRow.ID,
			RoomPlayerID: roomPlayerRow.ID,
			Role:         pgtype.Text{Valid: false},
		}
		gamePlayerRow, err := txQueries.CreateGamePlayer(ctx, createGamePlayerParams)
		if err != nil {
			return nil, fmt.Errorf("create game player: %w", err)
		}
		latestGame = dbGameToStoreGame(&latestGameRow)
		gamePlayer = dbGamePlayerToStoreGamePlayer(&gamePlayerRow, uuidToString(latestGameRow.ID))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	room := &Room{
		ID:        roomID,
		Code:      req.Code,
		Settings:  settings,
		CreatedAt: timestamptzToTime(roomRow.CreatedAt),
		UpdatedAt: timestamptzToTime(roomRow.UpdatedAt),
	}

	roomPlayer := &RoomPlayer{
		ID:          uuidToString(roomPlayerRow.ID),
		RoomID:      roomID,
		DisplayName: roomPlayerRow.DisplayName,
		IsHost:      roomPlayerRow.IsHost,
		CreatedAt:   timestamptzToTime(roomPlayerRow.CreatedAt),
	}

	return &JoinRoomResponse{
		Room:       room,
		RoomPlayer: roomPlayer,
		LatestGame: latestGame,
		GamePlayer: gamePlayer,
	}, nil
}

// GetRoomPlayerInRoom returns the room player with the given ID if they belong to the room identified by code.
// Returns (nil, error) if room not found or player not in room.
func (s *RoomStore) GetRoomPlayerInRoom(ctx context.Context, code string, roomPlayerID string) (*RoomPlayer, error) {
	roomRow, err := s.queries.GetRoomByCode(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("room not found")
		}
		return nil, fmt.Errorf("get room by code: %w", err)
	}
	roomUUID := roomRow.ID
	if _, err := stringToUUID(roomPlayerID); err != nil {
		return nil, fmt.Errorf("invalid room_player_id: %w", err)
	}
	players, err := s.queries.GetRoomPlayersByRoomId(ctx, roomUUID)
	if err != nil {
		return nil, fmt.Errorf("get room players: %w", err)
	}
	for i := range players {
		if uuidToString(players[i].ID) == roomPlayerID {
			rp := &players[i]
			return &RoomPlayer{
				ID:          uuidToString(rp.ID),
				RoomID:      uuidToString(rp.RoomID),
				DisplayName: rp.DisplayName,
				IsHost:      rp.IsHost,
				CreatedAt:   timestamptzToTime(rp.CreatedAt),
			}, nil
		}
	}
	return nil, fmt.Errorf("player not in room")
}

// GetRoom returns room info, latest game, and latest snapshot for the given room code.
func (s *RoomStore) GetRoom(ctx context.Context, code string) (*GetRoomResponse, error) {
	roomRow, err := s.queries.GetRoomByCode(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("room not found")
		}
		return nil, fmt.Errorf("get room by code: %w", err)
	}

	roomID := uuidToString(roomRow.ID)

	var settings map[string]interface{}
	if err := json.Unmarshal(roomRow.SettingsJson, &settings); err != nil {
		settings = make(map[string]interface{})
	}

	room := &Room{
		ID:        roomID,
		Code:      code,
		Settings:  settings,
		CreatedAt: timestamptzToTime(roomRow.CreatedAt),
		UpdatedAt: timestamptzToTime(roomRow.UpdatedAt),
	}

	roomUUID, err := stringToUUID(roomID)
	if err != nil {
		return nil, fmt.Errorf("convert room id to uuid: %w", err)
	}

	games, err := s.queries.GetGamesByRoomId(ctx, roomUUID)
	if err != nil {
		return nil, fmt.Errorf("get games by room: %w", err)
	}

	var latestGame *Game
	var snapshotMap map[string]interface{}

	if len(games) > 0 {
		latestGameRow := &games[0]
		latestGame = dbGameToStoreGame(latestGameRow)

		snapshotRow, err := s.queries.GetLatestGameStateSnapshotByGameId(ctx, latestGameRow.ID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, fmt.Errorf("get latest snapshot: %w", err)
		}
		if err == nil {
			if len(snapshotRow.StateJson) > 0 {
				if err := json.Unmarshal(snapshotRow.StateJson, &snapshotMap); err != nil {
					snapshotMap = make(map[string]interface{})
				}
			}
		}
	}

	return &GetRoomResponse{
		Room:                    room,
		LatestGame:              latestGame,
		LatestGameStateSnapshot: snapshotMap,
	}, nil
}

// dbGameToStoreGame converts db.Game to store.Game.
func dbGameToStoreGame(g *db.Game) *Game {
	if g == nil {
		return nil
	}
	var config map[string]interface{}
	_ = json.Unmarshal(g.ConfigJson, &config)
	if config == nil {
		config = make(map[string]interface{})
	}
	var endedAt *time.Time
	if g.EndedAt.Valid {
		t := timestamptzToTime(g.EndedAt)
		endedAt = &t
	}
	return &Game{
		ID:        uuidToString(g.ID),
		RoomID:    uuidToString(g.RoomID),
		Status:    g.Status,
		Config:    config,
		CreatedAt: timestamptzToTime(g.CreatedAt),
		EndedAt:   endedAt,
	}
}

// dbGamePlayerToStoreGamePlayer converts db.GamePlayer to store.GamePlayer.
func dbGamePlayerToStoreGamePlayer(gp *db.GamePlayer, gameID string) *GamePlayer {
	if gp == nil {
		return nil
	}
	var role *string
	if gp.Role.Valid {
		role = &gp.Role.String
	}
	var leftAt *time.Time
	if gp.LeftAt.Valid {
		t := timestamptzToTime(gp.LeftAt)
		leftAt = &t
	}
	return &GamePlayer{
		ID:           uuidToString(gp.ID),
		GameID:       gameID,
		RoomPlayerID: uuidToString(gp.RoomPlayerID),
		Role:         role,
		JoinedAt:     timestamptzToTime(gp.JoinedAt),
		LeftAt:       leftAt,
	}
}
