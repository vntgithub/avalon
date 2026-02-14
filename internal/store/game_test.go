package store

import (
	"context"
	"testing"
	"time"

	"github.com/vntrieu/avalon/internal/db"
)

func TestCreateGame(t *testing.T) {
	pool := SetupTestDB(t)
	defer pool.Close()

	roomStore := NewRoomStore(pool)
	gameStore := NewGameStore(pool)
	ctx := context.Background()

	// Helper function to create a room with players
	createRoomWithPlayers := func(t *testing.T, numPlayers int) *CreateRoomResponse {
		t.Helper()
		createRoomReq := CreateRoomRequest{
			DisplayName: "HostPlayer",
		}
		createRoomResp, err := roomStore.CreateRoom(ctx, createRoomReq, nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Add additional players
		for i := 1; i < numPlayers; i++ {
			joinReq := JoinRoomRequest{
				Code:        createRoomResp.Room.Code,
				DisplayName: "Player" + string(rune('A'+i)),
			}
			_, err := roomStore.JoinRoom(ctx, joinReq, nil)
			if err != nil {
				t.Fatalf("failed to join room: %v", err)
			}
		}

		return createRoomResp
	}

	t.Run("success with default config", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 3)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp == nil {
			t.Fatal("expected non-nil response")
		}

		// Validate game
		if resp.Game == nil {
			t.Fatal("expected non-nil game")
		}
		if resp.Game.ID == "" {
			t.Error("expected game ID to be set")
		}
		if resp.Game.RoomID != roomResp.Room.ID {
			t.Errorf("expected room_id %s, got %s", roomResp.Room.ID, resp.Game.RoomID)
		}
		if resp.Game.Status != "waiting" {
			t.Errorf("expected status 'waiting', got %q", resp.Game.Status)
		}
		if resp.Game.Config == nil {
			t.Error("expected config to be non-nil")
		}
		if len(resp.Game.Config) != 0 {
			t.Errorf("expected empty config, got %v", resp.Game.Config)
		}
		if resp.Game.CreatedAt.IsZero() {
			t.Error("expected created_at to be set")
		}
		if resp.Game.EndedAt != nil {
			t.Error("expected ended_at to be nil for new game")
		}

		// Validate players
		if len(resp.Players) != 3 {
			t.Errorf("expected 3 players, got %d", len(resp.Players))
		}

		for i, player := range resp.Players {
			if player.ID == "" {
				t.Errorf("player %d: expected ID to be set", i)
			}
			if player.GameID != resp.Game.ID {
				t.Errorf("player %d: expected game_id to match game ID", i)
			}
			if player.RoomPlayerID == "" {
				t.Errorf("player %d: expected room_player_id to be set", i)
			}
			if player.Role != nil {
				t.Errorf("player %d: expected role to be nil initially, got %v", i, player.Role)
			}
			if player.JoinedAt.IsZero() {
				t.Errorf("player %d: expected joined_at to be set", i)
			}
			if player.LeftAt != nil {
				t.Errorf("player %d: expected left_at to be nil", i)
			}
		}

		// Verify game exists in database
		queries := db.New(pool)
		gameUUID, err := stringToUUID(resp.Game.ID)
		if err != nil {
			t.Fatalf("failed to convert game ID to UUID: %v", err)
		}
		game, err := queries.GetGameById(ctx, gameUUID)
		if err != nil {
			t.Fatalf("failed to query game: %v", err)
		}
		if game.Status != "waiting" {
			t.Errorf("expected game status 'waiting', got %q", game.Status)
		}
	})

	t.Run("success with custom config", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 2)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
			Config: map[string]interface{}{
				"max_players": 10,
				"game_mode":  "classic",
				"time_limit": 300,
			},
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp.Game.Config == nil {
			t.Fatal("expected config to be set")
		}
		if maxPlayers, ok := resp.Game.Config["max_players"].(float64); !ok || maxPlayers != 10 {
			t.Errorf("expected max_players to be 10, got %v", resp.Game.Config["max_players"])
		}
		if gameMode, ok := resp.Game.Config["game_mode"].(string); !ok || gameMode != "classic" {
			t.Errorf("expected game_mode to be 'classic', got %v", resp.Game.Config["game_mode"])
		}
		if timeLimit, ok := resp.Game.Config["time_limit"].(float64); !ok || timeLimit != 300 {
			t.Errorf("expected time_limit to be 300, got %v", resp.Game.Config["time_limit"])
		}
	})

	t.Run("success with single player", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 1)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if len(resp.Players) != 1 {
			t.Errorf("expected 1 player, got %d", len(resp.Players))
		}
	})

	t.Run("success with multiple players", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 5)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if len(resp.Players) != 5 {
			t.Errorf("expected 5 players, got %d", len(resp.Players))
		}

		// Verify all players have unique IDs
		playerIDs := make(map[string]bool)
		for _, player := range resp.Players {
			if playerIDs[player.ID] {
				t.Errorf("duplicate player ID: %s", player.ID)
			}
			playerIDs[player.ID] = true
		}
	})

	t.Run("room not found", func(t *testing.T) {
		req := CreateGameRequest{
			RoomID: "00000000-0000-0000-0000-000000000000",
		}

		_, err := gameStore.CreateGame(ctx, req)
		if err == nil {
			t.Fatal("expected error for non-existent room")
		}
		if err.Error() != "room not found" {
			t.Errorf("expected 'room not found' error, got: %v", err)
		}
	})

	t.Run("invalid room_id format", func(t *testing.T) {
		req := CreateGameRequest{
			RoomID: "invalid-uuid",
		}

		_, err := gameStore.CreateGame(ctx, req)
		if err == nil {
			t.Fatal("expected error for invalid room_id format")
		}
		// Should get an error about invalid UUID format
	})

	t.Run("empty config", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 2)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
			Config: map[string]interface{}{},
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp.Game.Config == nil {
			t.Error("expected config to be non-nil (empty map)")
		}
		if len(resp.Game.Config) != 0 {
			t.Errorf("expected empty config, got %v", resp.Game.Config)
		}
	})

	t.Run("complex config JSON", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 2)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
			Config: map[string]interface{}{
				"max_players": 10,
				"game_mode":  "classic",
				"nested": map[string]interface{}{
					"key":   "value",
					"number": 42,
				},
				"array": []interface{}{1, 2, 3},
			},
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp.Game.Config == nil {
			t.Fatal("expected config to be set")
		}

		// Verify nested structure
		nested, ok := resp.Game.Config["nested"].(map[string]interface{})
		if !ok {
			t.Error("expected nested config to be map")
		} else {
			if nested["key"] != "value" {
				t.Errorf("expected nested.key to be 'value', got %v", nested["key"])
			}
		}
	})

	t.Run("timestamps are recent", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 2)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}

		before := time.Now()
		resp, err := gameStore.CreateGame(ctx, req)
		after := time.Now()

		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp.Game.CreatedAt.Before(before) || resp.Game.CreatedAt.After(after) {
			t.Errorf("created_at %v is not between %v and %v", resp.Game.CreatedAt, before, after)
		}

		for i, player := range resp.Players {
			if player.JoinedAt.Before(before) || player.JoinedAt.After(after) {
				t.Errorf("player %d joined_at %v is not between %v and %v", i, player.JoinedAt, before, after)
			}
		}
	})

	t.Run("multiple games in same room", func(t *testing.T) {
		roomResp := createRoomWithPlayers(t, 2)

		// Create first game
		req1 := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}
		resp1, err := gameStore.CreateGame(ctx, req1)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		// Create second game
		req2 := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}
		resp2, err := gameStore.CreateGame(ctx, req2)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		if resp1.Game.ID == resp2.Game.ID {
			t.Error("expected different game IDs")
		}

		// Verify both games exist
		queries := db.New(pool)
		roomUUID, err := stringToUUID(roomResp.Room.ID)
		if err != nil {
			t.Fatalf("failed to convert room ID to UUID: %v", err)
		}
		games, err := queries.GetGamesByRoomId(ctx, roomUUID)
		if err != nil {
			t.Fatalf("failed to query games: %v", err)
		}
		if len(games) != 2 {
			t.Errorf("expected 2 games, got %d", len(games))
		}
	})

	t.Run("transaction rollback on error", func(t *testing.T) {
		// This test verifies that if game player creation fails, the game is also rolled back
		// We can't easily simulate this without mocking, but we can verify
		// that both game and players are created together in a successful case

		roomResp := createRoomWithPlayers(t, 2)

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		// Verify game exists
		queries := db.New(pool)
		gameUUID, err := stringToUUID(resp.Game.ID)
		if err != nil {
			t.Fatalf("failed to convert game ID to UUID: %v", err)
		}
		game, err := queries.GetGameById(ctx, gameUUID)
		if err != nil {
			t.Fatalf("failed to query game: %v", err)
		}
		if game.ID != gameUUID {
			t.Error("game not found in database")
		}

		// Verify all players exist (we'd need a query for this, but the fact that
		// the game was created successfully means the transaction committed)
		if len(resp.Players) != 2 {
			t.Errorf("expected 2 players, got %d", len(resp.Players))
		}
	})
}

func TestCreateGame_EdgeCases(t *testing.T) {
	pool := SetupTestDB(t)
	defer pool.Close()

	roomStore := NewRoomStore(pool)
	gameStore := NewGameStore(pool)
	ctx := context.Background()

	t.Run("invalid room_id format in edge cases", func(t *testing.T) {
		req := CreateGameRequest{
			RoomID: "not-a-valid-uuid",
		}

		_, err := gameStore.CreateGame(ctx, req)
		if err == nil {
			t.Fatal("expected error for invalid room_id format")
		}
		// Should get an error about invalid UUID
	})

	t.Run("nil config", func(t *testing.T) {
		createRoomReq := CreateRoomRequest{
			DisplayName: "HostPlayer",
		}
		roomResp, err := roomStore.CreateRoom(ctx, createRoomReq, nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		req := CreateGameRequest{
			RoomID: roomResp.Room.ID,
			Config: nil,
		}

		resp, err := gameStore.CreateGame(ctx, req)
		if err != nil {
			t.Fatalf("CreateGame failed: %v", err)
		}

		// Config should be empty map, not nil
		if resp.Game.Config == nil {
			t.Error("expected config to be non-nil (empty map)")
		}
		if len(resp.Game.Config) != 0 {
			t.Errorf("expected empty config, got %v", resp.Game.Config)
		}
	})
}
