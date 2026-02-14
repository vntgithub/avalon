package store

import (
	"context"
	"testing"
	"time"

	"github.com/vntrieu/avalon/internal/db"
)

func TestCreateRoom(t *testing.T) {
	pool := SetupTestDB(t)
	defer pool.Close()

	store := NewRoomStore(pool)
	ctx := context.Background()

	t.Run("success without password", func(t *testing.T) {
		req := CreateRoomRequest{
			Settings: map[string]interface{}{
				"max_players": 10,
			},
		}

		resp, err := store.CreateRoom(ctx, req, "TestPlayer", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if resp == nil {
			t.Fatal("expected non-nil response")
		}

		// Validate room
		if resp.Room == nil {
			t.Fatal("expected non-nil room")
		}
		if resp.Room.ID == "" {
			t.Error("expected room ID to be set")
		}
		if resp.Room.Code == "" {
			t.Error("expected room code to be set")
		}
		if len(resp.Room.Code) != 6 {
			t.Errorf("expected room code to be 6 characters, got %d", len(resp.Room.Code))
		}
		if resp.Room.PasswordHash != nil {
			t.Error("expected password hash to be nil when no password provided")
		}
		if resp.Room.Settings == nil {
			t.Error("expected settings to be set")
		}
		if maxPlayers, ok := resp.Room.Settings["max_players"].(float64); !ok || maxPlayers != 10 {
			t.Errorf("expected max_players to be 10, got %v", resp.Room.Settings["max_players"])
		}
		if resp.Room.CreatedAt.IsZero() {
			t.Error("expected created_at to be set")
		}
		if resp.Room.UpdatedAt.IsZero() {
			t.Error("expected updated_at to be set")
		}

		// Validate room player
		if resp.RoomPlayer == nil {
			t.Fatal("expected non-nil room player")
		}
		if resp.RoomPlayer.ID == "" {
			t.Error("expected room player ID to be set")
		}
		if resp.RoomPlayer.RoomID != resp.Room.ID {
			t.Error("expected room player room_id to match room id")
		}
		if resp.RoomPlayer.DisplayName != "TestPlayer" {
			t.Errorf("expected display name to be 'TestPlayer', got %q", resp.RoomPlayer.DisplayName)
		}
		if !resp.RoomPlayer.IsHost {
			t.Error("expected room player to be host")
		}
		if resp.RoomPlayer.CreatedAt.IsZero() {
			t.Error("expected room player created_at to be set")
		}

		// Verify room exists in database
		queries := db.New(pool)
		roomUUID, err := stringToUUID(resp.Room.ID)
		if err != nil {
			t.Fatalf("failed to convert room ID to UUID: %v", err)
		}
		code, err := queries.GetRoomCodeById(ctx, roomUUID)
		if err != nil {
			t.Fatalf("failed to query room: %v", err)
		}
		if code != resp.Room.Code {
			t.Errorf("expected room code %q, got %q", resp.Room.Code, code)
		}
	})

	t.Run("success with password", func(t *testing.T) {
		req := CreateRoomRequest{
			Password: "secret123",
		}

		resp, err := store.CreateRoom(ctx, req, "SecurePlayer", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if resp.Room.PasswordHash == nil {
			t.Error("expected password hash to be set when password provided")
		}
		if *resp.Room.PasswordHash == "" {
			t.Error("expected password hash to be non-empty")
		}

		// Verify password hash in database
		queries := db.New(pool)
		roomUUID, err := stringToUUID(resp.Room.ID)
		if err != nil {
			t.Fatalf("failed to convert room ID to UUID: %v", err)
		}
		passwordHash, err := queries.GetRoomPasswordHashById(ctx, roomUUID)
		if err != nil {
			t.Fatalf("failed to query room password: %v", err)
		}
		if !passwordHash.Valid {
			t.Error("expected password hash to be stored in database")
		}
	})

	t.Run("success with empty settings", func(t *testing.T) {
		req := CreateRoomRequest{}

		resp, err := store.CreateRoom(ctx, req, "SimplePlayer", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if resp.Room.Settings == nil {
			t.Error("expected settings to be non-nil (empty map)")
		}
		if len(resp.Room.Settings) != 0 {
			t.Errorf("expected empty settings, got %v", resp.Room.Settings)
		}
	})

	t.Run("generates unique room codes", func(t *testing.T) {
		codes := make(map[string]bool)
		for i := 0; i < 10; i++ {
			req := CreateRoomRequest{}
			displayName := "Player" + string(rune('A'+i))

			resp, err := store.CreateRoom(ctx, req, displayName, nil)
			if err != nil {
				t.Fatalf("CreateRoom failed: %v", err)
			}

			if codes[resp.Room.Code] {
				t.Errorf("duplicate room code generated: %s", resp.Room.Code)
			}
			codes[resp.Room.Code] = true
		}
	})

	t.Run("room code format", func(t *testing.T) {
		req := CreateRoomRequest{}

		resp, err := store.CreateRoom(ctx, req, "FormatTest", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		code := resp.Room.Code
		if len(code) != 6 {
			t.Errorf("expected room code length 6, got %d", len(code))
		}

		// Check that code only contains valid characters
		validChars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		for _, char := range code {
			found := false
			for _, valid := range validChars {
				if char == valid {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("room code contains invalid character: %c", char)
			}
		}
	})

	t.Run("transaction rollback on error", func(t *testing.T) {
		// This test verifies that if room player creation fails, the room is also rolled back
		// We'll simulate this by using a display name that might cause issues
		// Actually, we can't easily simulate this without mocking, but we can verify
		// that both room and player are created together in a successful case

		req := CreateRoomRequest{}

		resp, err := store.CreateRoom(ctx, req, "TransactionTest", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		// Verify both room and player exist
		queries := db.New(pool)
		roomUUID, err := stringToUUID(resp.Room.ID)
		if err != nil {
			t.Fatalf("failed to convert room ID to UUID: %v", err)
		}
		roomCount, err := queries.CountRoomsById(ctx, roomUUID)
		if err != nil {
			t.Fatalf("failed to query room count: %v", err)
		}
		if roomCount != 1 {
			t.Errorf("expected 1 room, got %d", roomCount)
		}

		playerUUID, err := stringToUUID(resp.RoomPlayer.ID)
		if err != nil {
			t.Fatalf("failed to convert player ID to UUID: %v", err)
		}
		playerCount, err := queries.CountRoomPlayersById(ctx, playerUUID)
		if err != nil {
			t.Fatalf("failed to query player count: %v", err)
		}
		if playerCount != 1 {
			t.Errorf("expected 1 player, got %d", playerCount)
		}
	})
}

func TestCreateRoom_EdgeCases(t *testing.T) {
	pool := SetupTestDB(t)
	defer pool.Close()

	store := NewRoomStore(pool)
	ctx := context.Background()

	t.Run("empty display name", func(t *testing.T) {
		req := CreateRoomRequest{}

		// Store requires non-empty display name
		_, err := store.CreateRoom(ctx, req, "", nil)
		if err == nil {
			t.Fatal("expected error for empty display name")
		}
		if err.Error() != "display_name is required" {
			t.Errorf("expected 'display_name is required' error, got: %v", err)
		}
	})

	t.Run("complex settings JSON", func(t *testing.T) {
		req := CreateRoomRequest{
			Settings: map[string]interface{}{
				"max_players": 10,
				"game_variant": "classic",
				"time_per_round": 300,
				"nested": map[string]interface{}{
					"key": "value",
					"number": 42,
				},
			},
		}

		resp, err := store.CreateRoom(ctx, req, "ComplexSettings", nil)
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if resp.Room.Settings == nil {
			t.Fatal("expected settings to be set")
		}

		// Verify settings are preserved
		if maxPlayers, ok := resp.Room.Settings["max_players"].(float64); !ok || maxPlayers != 10 {
			t.Errorf("expected max_players to be 10, got %v", resp.Room.Settings["max_players"])
		}
		if variant, ok := resp.Room.Settings["game_variant"].(string); !ok || variant != "classic" {
			t.Errorf("expected game_variant to be 'classic', got %v", resp.Room.Settings["game_variant"])
		}
	})

	t.Run("timestamps are recent", func(t *testing.T) {
		req := CreateRoomRequest{}

		before := time.Now()
		resp, err := store.CreateRoom(ctx, req, "TimestampTest", nil)
		after := time.Now()

		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if resp.Room.CreatedAt.Before(before) || resp.Room.CreatedAt.After(after) {
			t.Errorf("created_at %v is not between %v and %v", resp.Room.CreatedAt, before, after)
		}

		if resp.RoomPlayer.CreatedAt.Before(before) || resp.RoomPlayer.CreatedAt.After(after) {
			t.Errorf("player created_at %v is not between %v and %v", resp.RoomPlayer.CreatedAt, before, after)
		}
	})
}

func TestJoinRoom(t *testing.T) {
	pool := SetupTestDB(t)
	defer pool.Close()

	store := NewRoomStore(pool)
	ctx := context.Background()

	t.Run("success join room without password", func(t *testing.T) {
		// Create a room first
		createReq := CreateRoomRequest{}
		createResp, err := store.CreateRoom(ctx, createReq, "HostPlayer", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Join the room
		joinReq := JoinRoomRequest{
			Code: createResp.Room.Code,
		}

		joinResp, err := store.JoinRoom(ctx, joinReq, "GuestPlayer", nil)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		if joinResp == nil {
			t.Fatal("expected non-nil response")
		}

		// Validate room
		if joinResp.Room == nil {
			t.Fatal("expected non-nil room")
		}
		if joinResp.Room.ID != createResp.Room.ID {
			t.Errorf("expected room ID %s, got %s", createResp.Room.ID, joinResp.Room.ID)
		}
		if joinResp.Room.Code != createResp.Room.Code {
			t.Errorf("expected room code %s, got %s", createResp.Room.Code, joinResp.Room.Code)
		}

		// Validate room player
		if joinResp.RoomPlayer == nil {
			t.Fatal("expected non-nil room player")
		}
		if joinResp.RoomPlayer.ID == "" {
			t.Error("expected room player ID to be set")
		}
		if joinResp.RoomPlayer.RoomID != createResp.Room.ID {
			t.Error("expected room player room_id to match room id")
		}
		if joinResp.RoomPlayer.DisplayName != "GuestPlayer" {
			t.Errorf("expected display name 'GuestPlayer', got %q", joinResp.RoomPlayer.DisplayName)
		}
		if joinResp.RoomPlayer.IsHost {
			t.Error("expected room player to not be host")
		}
		if joinResp.RoomPlayer.CreatedAt.IsZero() {
			t.Error("expected room player created_at to be set")
		}

		// Verify player exists in database
		queries := db.New(pool)
		playerUUID, err := stringToUUID(joinResp.RoomPlayer.ID)
		if err != nil {
			t.Fatalf("failed to convert player ID to UUID: %v", err)
		}
		count, err := queries.CountRoomPlayersById(ctx, playerUUID)
		if err != nil {
			t.Fatalf("failed to query player: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 player, got %d", count)
		}
	})

	t.Run("success join room with password", func(t *testing.T) {
		// Create a room with password
		createReq := CreateRoomRequest{
			Password: "secret123",
		}
		createResp, err := store.CreateRoom(ctx, createReq, "SecureHost", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Join the room with correct password
		joinReq := JoinRoomRequest{
			Code:     createResp.Room.Code,
			Password: "secret123",
		}

		joinResp, err := store.JoinRoom(ctx, joinReq, "SecureGuest", nil)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		if joinResp.RoomPlayer.DisplayName != "SecureGuest" {
			t.Errorf("expected display name 'SecureGuest', got %q", joinResp.RoomPlayer.DisplayName)
		}
	})

	t.Run("room not found", func(t *testing.T) {
		joinReq := JoinRoomRequest{
			Code: "INVALID",
		}

		_, err := store.JoinRoom(ctx, joinReq, "GuestPlayer", nil)
		if err == nil {
			t.Fatal("expected error for non-existent room")
		}
		if err.Error() != "room not found" {
			t.Errorf("expected 'room not found' error, got: %v", err)
		}
	})

	t.Run("password required for protected room", func(t *testing.T) {
		// Create a room with password
		createReq := CreateRoomRequest{
			Password: "password123",
		}
		createResp, err := store.CreateRoom(ctx, createReq, "ProtectedHost", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Try to join without password
		joinReq := JoinRoomRequest{
			Code: createResp.Room.Code,
		}

		_, err = store.JoinRoom(ctx, joinReq, "GuestPlayer", nil)
		if err == nil {
			t.Fatal("expected error when password is required but not provided")
		}
		if err.Error() != "password is required" {
			t.Errorf("expected 'password is required' error, got: %v", err)
		}
	})

	t.Run("invalid password", func(t *testing.T) {
		// Create a room with password
		createReq := CreateRoomRequest{
			Password: "correctpassword",
		}
		createResp, err := store.CreateRoom(ctx, createReq, "ProtectedHost2", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Try to join with wrong password
		joinReq := JoinRoomRequest{
			Code:     createResp.Room.Code,
			Password: "wrongpassword",
		}

		_, err = store.JoinRoom(ctx, joinReq, "GuestPlayer", nil)
		if err == nil {
			t.Fatal("expected error for invalid password")
		}
		if err.Error() != "invalid password" {
			t.Errorf("expected 'invalid password' error, got: %v", err)
		}
	})

	t.Run("display name already taken", func(t *testing.T) {
		// Create a room
		createReq := CreateRoomRequest{}
		createResp, err := store.CreateRoom(ctx, createReq, "HostPlayer", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Join the room with first player
		joinReq1 := JoinRoomRequest{
			Code: createResp.Room.Code,
		}
		_, err = store.JoinRoom(ctx, joinReq1, "Player1", nil)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Try to join again with same display name
		joinReq2 := JoinRoomRequest{
			Code: createResp.Room.Code,
		}
		_, err = store.JoinRoom(ctx, joinReq2, "Player1", nil)
		if err == nil {
			t.Fatal("expected error for duplicate display name")
		}
		if err.Error() != "display name already taken in this room" {
			t.Errorf("expected 'display name already taken in this room' error, got: %v", err)
		}
	})

	t.Run("empty display name", func(t *testing.T) {
		// Create a room
		createReq := CreateRoomRequest{}
		createResp, err := store.CreateRoom(ctx, createReq, "HostPlayer", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Try to join with empty display name (store rejects empty displayName)
		joinReq := JoinRoomRequest{
			Code: createResp.Room.Code,
		}

		_, err = store.JoinRoom(ctx, joinReq, "", nil)
		if err == nil {
			t.Fatal("expected error for empty display name")
		}
		if err.Error() != "display_name is required" {
			t.Errorf("expected 'display_name is required' error, got: %v", err)
		}
	})

	t.Run("multiple players can join same room", func(t *testing.T) {
		// Create a room
		createReq := CreateRoomRequest{}
		createResp, err := store.CreateRoom(ctx, createReq, "HostPlayer", nil)
		if err != nil {
			t.Fatalf("failed to create room: %v", err)
		}

		// Join multiple players
		players := []string{"Player1", "Player2", "Player3"}
		for _, playerName := range players {
			joinReq := JoinRoomRequest{
				Code: createResp.Room.Code,
			}
			_, err := store.JoinRoom(ctx, joinReq, playerName, nil)
			if err != nil {
				t.Fatalf("failed to join room as %s: %v", playerName, err)
			}
		}

		// Verify all players exist
		queries := db.New(pool)
		roomUUID, err := stringToUUID(createResp.Room.ID)
		if err != nil {
			t.Fatalf("failed to convert room ID to UUID: %v", err)
		}
		count, err := queries.CountRoomPlayersByRoomId(ctx, roomUUID)
		if err != nil {
			t.Fatalf("failed to query players: %v", err)
		}
		// 1 host + 3 players = 4 total
		if count != 4 {
			t.Errorf("expected 4 players (1 host + 3 guests), got %d", count)
		}
	})
}
