package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vntrieu/avalon/internal/httpapi/handler"
	"github.com/vntrieu/avalon/internal/store"
)

func setupTestHandler(t *testing.T) (*handler.RoomHandler, *pgxpool.Pool) {
	t.Helper()
	pool := store.SetupTestDB(t)
	roomStore := store.NewRoomStore(pool)
	h := handler.NewRoomHandler(roomStore, nil)
	return h, pool
}

func TestCreateRoomHandler(t *testing.T) {
	t.Run("success without password", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()

		reqBody := map[string]interface{}{
			"display_name": "TestPlayer",
			"settings": map[string]interface{}{
				"max_players": 10,
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
		}
		var resp store.CreateRoomResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Room == nil || resp.Room.Code == "" {
			t.Error("expected non-nil room with code")
		}
		if resp.RoomPlayer == nil || resp.RoomPlayer.DisplayName != "TestPlayer" || !resp.RoomPlayer.IsHost {
			t.Error("expected host room player")
		}
		if w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", w.Header().Get("Content-Type"))
		}
	})

	t.Run("success with password", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		body, _ := json.Marshal(map[string]interface{}{"display_name": "SecurePlayer", "password": "secret123"})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", w.Code)
		}
		var resp store.CreateRoomResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Room == nil || resp.RoomPlayer == nil {
			t.Fatal("expected room and player")
		}
	})

	t.Run("missing display_name", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		body, _ := json.Marshal(map[string]interface{}{"password": "secret123"})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if w.Body.String() != "display_name is required\n" {
			t.Errorf("unexpected body: %q", w.Body.String())
		}
	})

	t.Run("empty display_name", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		body, _ := json.Marshal(map[string]interface{}{"display_name": ""})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if w.Body.String() != "invalid request body\n" {
			t.Errorf("unexpected body: %q", w.Body.String())
		}
	})

	t.Run("wrong HTTP method", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		body, _ := json.Marshal(map[string]interface{}{"display_name": "TestPlayer"})
		req := httptest.NewRequest(http.MethodGet, "/api/rooms", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", w.Code)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("complex settings", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		reqBody := map[string]interface{}{
			"display_name": "ComplexPlayer",
			"settings": map[string]interface{}{
				"max_players": 10, "game_variant": "classic", "time_per_round": 300,
				"nested": map[string]interface{}{"key": "value", "number": 42},
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateRoom(w, req)
		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", w.Code)
		}
		var resp store.CreateRoomResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Room.Settings == nil {
			t.Fatal("expected settings")
		}
		if maxPlayers, ok := resp.Room.Settings["max_players"].(float64); !ok || maxPlayers != 10 {
			t.Errorf("expected max_players 10, got %v", resp.Room.Settings["max_players"])
		}
	})
}

func chiCtxWithCode(code string) func(*http.Request) *http.Request {
	return func(r *http.Request) *http.Request {
		ctx := context.WithValue(r.Context(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
		})
		return r.WithContext(ctx)
	}
}

func TestJoinRoomHandler(t *testing.T) {
	t.Run("success join room without password", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "HostPlayer"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		var createResp store.CreateRoomResponse
		json.NewDecoder(createW.Body).Decode(&createResp)

		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "GuestPlayer"})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode(createResp.Room.Code)(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)

		if joinW.Code != http.StatusOK {
			t.Errorf("expected 200, got %d body=%s", joinW.Code, joinW.Body.String())
		}
		var joinResp store.JoinRoomResponse
		if err := json.NewDecoder(joinW.Body).Decode(&joinResp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if joinResp.Room == nil || joinResp.Room.Code != createResp.Room.Code {
			t.Error("expected room in response")
		}
		if joinResp.RoomPlayer == nil || joinResp.RoomPlayer.DisplayName != "GuestPlayer" || joinResp.RoomPlayer.IsHost {
			t.Error("expected guest player")
		}
		if joinW.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", joinW.Header().Get("Content-Type"))
		}
	})

	t.Run("success join room with password", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "SecureHost", "password": "secret123"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		var createResp store.CreateRoomResponse
		json.NewDecoder(createW.Body).Decode(&createResp)

		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "SecureGuest", "password": "secret123"})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode(createResp.Room.Code)(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", joinW.Code)
		}
		var joinResp store.JoinRoomResponse
		json.NewDecoder(joinW.Body).Decode(&joinResp)
		if joinResp.RoomPlayer.DisplayName != "SecureGuest" {
			t.Errorf("expected SecureGuest, got %q", joinResp.RoomPlayer.DisplayName)
		}
	})

	t.Run("room not found", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "GuestPlayer"})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/INVALID/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode("INVALID")(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", joinW.Code)
		}
		if joinW.Body.String() != "room not found\n" {
			t.Errorf("unexpected body: %q", joinW.Body.String())
		}
	})

	t.Run("password required for protected room", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "ProtectedHost", "password": "password123"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		var createResp store.CreateRoomResponse
		json.NewDecoder(createW.Body).Decode(&createResp)

		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "GuestPlayer"})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode(createResp.Room.Code)(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", joinW.Code)
		}
		if joinW.Body.String() != "password is required\n" {
			t.Errorf("unexpected body: %q", joinW.Body.String())
		}
	})

	t.Run("invalid password", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "ProtectedHost2", "password": "correctpassword"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		var createResp store.CreateRoomResponse
		json.NewDecoder(createW.Body).Decode(&createResp)

		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "GuestPlayer", "password": "wrongpassword"})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode(createResp.Room.Code)(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", joinW.Code)
		}
		if joinW.Body.String() != "invalid password\n" {
			t.Errorf("unexpected body: %q", joinW.Body.String())
		}
	})

	t.Run("display name already taken", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "HostPlayer"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		var createResp store.CreateRoomResponse
		json.NewDecoder(createW.Body).Decode(&createResp)

		joinBody1, _ := json.Marshal(map[string]interface{}{"display_name": "Player1"})
		joinReq1 := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody1))
		joinReq1.Header.Set("Content-Type", "application/json")
		joinReq1 = chiCtxWithCode(createResp.Room.Code)(joinReq1)
		joinW1 := httptest.NewRecorder()
		h.JoinRoom(joinW1, joinReq1)

		joinBody2, _ := json.Marshal(map[string]interface{}{"display_name": "Player1"})
		joinReq2 := httptest.NewRequest(http.MethodPost, "/api/rooms/"+createResp.Room.Code+"/join", bytes.NewReader(joinBody2))
		joinReq2.Header.Set("Content-Type", "application/json")
		joinReq2 = chiCtxWithCode(createResp.Room.Code)(joinReq2)
		joinW2 := httptest.NewRecorder()
		h.JoinRoom(joinW2, joinReq2)
		if joinW2.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d", joinW2.Code)
		}
		if joinW2.Body.String() != "display name already taken in this room\n" {
			t.Errorf("unexpected body: %q", joinW2.Body.String())
		}
	})

	t.Run("missing display_name", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		joinBody, _ := json.Marshal(map[string]interface{}{})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/ABC123/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode("ABC123")(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", joinW.Code)
		}
		if joinW.Body.String() != "display_name is required\n" {
			t.Errorf("unexpected body: %q", joinW.Body.String())
		}
	})

	t.Run("empty display_name", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": ""})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/ABC123/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode("ABC123")(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", joinW.Code)
		}
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/ABC123/join", bytes.NewReader([]byte("invalid json")))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = chiCtxWithCode("ABC123")(joinReq)
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", joinW.Code)
		}
		if joinW.Body.String() != "invalid request body\n" {
			t.Errorf("unexpected body: %q", joinW.Body.String())
		}
	})

	t.Run("wrong HTTP method", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		joinBody, _ := json.Marshal(map[string]interface{}{"display_name": "GuestPlayer"})
		joinReq := httptest.NewRequest(http.MethodGet, "/api/rooms/ABC123/join", bytes.NewReader(joinBody))
		joinW := httptest.NewRecorder()
		h.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", joinW.Code)
		}
	})
}

func TestGetRoomHandler(t *testing.T) {
	t.Run("success returns room and latest game", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		createBody, _ := json.Marshal(map[string]interface{}{"display_name": "Host"})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createW := httptest.NewRecorder()
		h.CreateRoom(createW, createReq)
		if createW.Code != http.StatusCreated {
			t.Fatalf("create room: expected 201, got %d", createW.Code)
		}
		var createResp store.CreateRoomResponse
		if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		code := createResp.Room.Code

		getReq := httptest.NewRequest(http.MethodGet, "/api/rooms/"+code, nil)
		getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
		}))
		getW := httptest.NewRecorder()
		h.GetRoom(getW, getReq)

		if getW.Code != http.StatusOK {
			t.Errorf("expected 200, got %d body=%s", getW.Code, getW.Body.String())
		}
		var getResp store.GetRoomResponse
		if err := json.NewDecoder(getW.Body).Decode(&getResp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if getResp.Room == nil || getResp.Room.Code != code {
			t.Error("expected room in response")
		}
		if getResp.LatestGame == nil {
			t.Error("expected non-nil latest_game")
		}
	})

	t.Run("not found for unknown code", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		getReq := httptest.NewRequest(http.MethodGet, "/api/rooms/ZZZZ99", nil)
		getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{"ZZZZ99"}},
		}))
		getW := httptest.NewRecorder()
		h.GetRoom(getW, getReq)
		if getW.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", getW.Code)
		}
	})

	t.Run("invalid room code format", func(t *testing.T) {
		h, pool := setupTestHandler(t)
		defer pool.Close()
		for _, code := range []string{"x", "12345", "1234567", "!!!!!!"} {
			getReq := httptest.NewRequest(http.MethodGet, "/api/rooms/"+code, nil)
			getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, &chi.Context{
				URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
			}))
			getW := httptest.NewRecorder()
			h.GetRoom(getW, getReq)
			if getW.Code != http.StatusBadRequest {
				t.Errorf("code %q: expected 400, got %d", code, getW.Code)
			}
		}
	})
}
