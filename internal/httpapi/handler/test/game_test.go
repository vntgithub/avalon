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

func setupTestGameHandler(t *testing.T) (*handler.GameHandler, *handler.RoomHandler, *store.UserStore, *store.User, *pgxpool.Pool) {
	t.Helper()
	pool := store.SetupTestDB(t)
	roomStore := store.NewRoomStore(pool)
	gameStore := store.NewGameStore(pool)
	userStore := store.NewUserStore(pool)
	hostUser, err := userStore.CreateUser(context.Background(), "host@example.com", "password123", "Host")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	roomHandler := handler.NewRoomHandler(roomStore, userStore, nil)
	gameHandler := handler.NewGameHandler(gameStore, roomStore, nil)
	return gameHandler, roomHandler, userStore, hostUser, pool
}

func requestWithUserIDGame(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, handler.UserIDContextKey, userID)
	return r.WithContext(ctx)
}

func requestWithCodeChi(r *http.Request, code string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams = chi.RouteParams{Keys: []string{"code"}, Values: []string{code}}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
}

func TestCreateGameHandler(t *testing.T) {
	t.Run("401 when no user token", func(t *testing.T) {
		gameHandler, roomHandler, _, hostUser, pool := setupTestGameHandler(t)
		defer pool.Close()

		createBody, _ := json.Marshal(map[string]interface{}{})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq = requestWithUserIDGame(createReq, hostUser.ID)
		createW := httptest.NewRecorder()
		roomHandler.CreateRoom(createW, createReq)
		if createW.Code != http.StatusCreated {
			t.Fatalf("create room: expected 201, got %d", createW.Code)
		}
		var createResp store.CreateRoomResponse
		if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		code := createResp.Room.Code

		body, _ := json.Marshal(map[string]interface{}{})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/games", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = requestWithCodeChi(req, code)
		// no user in context -> 401
		w := httptest.NewRecorder()
		gameHandler.CreateGame(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("403 when non-host user", func(t *testing.T) {
		gameHandler, roomHandler, userStore, hostUser, pool := setupTestGameHandler(t)
		defer pool.Close()
		guestUser, err := userStore.CreateUser(context.Background(), "player2@example.com", "password123", "Player2")
		if err != nil {
			t.Fatalf("create guest user: %v", err)
		}

		createBody, _ := json.Marshal(map[string]interface{}{})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq = requestWithUserIDGame(createReq, hostUser.ID)
		createW := httptest.NewRecorder()
		roomHandler.CreateRoom(createW, createReq)
		if createW.Code != http.StatusCreated {
			t.Fatalf("create room: expected 201, got %d", createW.Code)
		}
		var createResp store.CreateRoomResponse
		if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		code := createResp.Room.Code

		joinBody, _ := json.Marshal(map[string]interface{}{})
		joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/join", bytes.NewReader(joinBody))
		joinReq.Header.Set("Content-Type", "application/json")
		joinReq = requestWithCodeChi(joinReq, code)
		joinReq = requestWithUserIDGame(joinReq, guestUser.ID)
		joinW := httptest.NewRecorder()
		roomHandler.JoinRoom(joinW, joinReq)
		if joinW.Code != http.StatusOK {
			t.Fatalf("join room: expected 200, got %d", joinW.Code)
		}

		body, _ := json.Marshal(map[string]interface{}{})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/games", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = requestWithCodeChi(req, code)
		req = requestWithUserIDGame(req, guestUser.ID) // non-host -> 403
		w := httptest.NewRecorder()
		gameHandler.CreateGame(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("201 when host creates game", func(t *testing.T) {
		gameHandler, roomHandler, _, hostUser, pool := setupTestGameHandler(t)
		defer pool.Close()

		createBody, _ := json.Marshal(map[string]interface{}{})
		createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq = requestWithUserIDGame(createReq, hostUser.ID)
		createW := httptest.NewRecorder()
		roomHandler.CreateRoom(createW, createReq)
		if createW.Code != http.StatusCreated {
			t.Fatalf("create room: expected 201, got %d", createW.Code)
		}
		var createResp store.CreateRoomResponse
		if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		code := createResp.Room.Code

		body, _ := json.Marshal(map[string]interface{}{})
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/games", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = requestWithCodeChi(req, code)
		req = requestWithUserIDGame(req, hostUser.ID)
		w := httptest.NewRecorder()
		gameHandler.CreateGame(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d body=%s", w.Code, w.Body.String())
		}
		var resp store.CreateGameResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Game == nil {
			t.Fatal("expected non-nil game")
		}
		if resp.Game.Status != "waiting" {
			t.Errorf("expected game status waiting, got %q", resp.Game.Status)
		}
		if resp.LatestGameStateSnapshot == nil {
			t.Error("expected non-nil latest_game_state_snapshot")
		}
		if resp.LatestGameStateSnapshot != nil {
			if phase, _ := resp.LatestGameStateSnapshot["phase"].(string); phase != "lobby" {
				t.Errorf("expected snapshot phase lobby, got %v", resp.LatestGameStateSnapshot["phase"])
			}
		}
	})
}

// TestRoomAndGameLifecycle_Integration runs full flow: create room → get room → join → get room → host creates game → get room returns new game.
func TestRoomAndGameLifecycle_Integration(t *testing.T) {
	gameHandler, roomHandler, userStore, hostUser, pool := setupTestGameHandler(t)
	defer pool.Close()
	guestUser, err := userStore.CreateUser(context.Background(), "player2@example.com", "password123", "Player2")
	if err != nil {
		t.Fatalf("create guest user: %v", err)
	}

	// 1. Create room
	createBody, _ := json.Marshal(map[string]interface{}{})
	createReq := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = requestWithUserIDGame(createReq, hostUser.ID)
	createW := httptest.NewRecorder()
	roomHandler.CreateRoom(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create room: expected 201, got %d", createW.Code)
	}
	var createResp store.CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	code := createResp.Room.Code

	// 2. GET room — assert room and latest game/snapshot
	getReq1 := httptest.NewRequest(http.MethodGet, "/api/rooms/"+code, nil)
	getReq1 = getReq1.WithContext(context.WithValue(getReq1.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
	}))
	getW1 := httptest.NewRecorder()
	roomHandler.GetRoom(getW1, getReq1)
	if getW1.Code != http.StatusOK {
		t.Fatalf("get room: expected 200, got %d", getW1.Code)
	}
	var getResp1 store.GetRoomResponse
	if err := json.NewDecoder(getW1.Body).Decode(&getResp1); err != nil {
		t.Fatalf("decode get room: %v", err)
	}
	if getResp1.Room == nil || getResp1.Room.Code != code {
		t.Error("expected room in get response")
	}
	if getResp1.LatestGame == nil {
		t.Error("expected latest_game after create (initial game created with room)")
	}

	// 3. Join room
	joinBody, _ := json.Marshal(map[string]interface{}{})
	joinReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/join", bytes.NewReader(joinBody))
	joinReq.Header.Set("Content-Type", "application/json")
	joinReq = requestWithCodeChi(joinReq, code)
	joinReq = requestWithUserIDGame(joinReq, guestUser.ID)
	joinW := httptest.NewRecorder()
	roomHandler.JoinRoom(joinW, joinReq)
	if joinW.Code != http.StatusOK {
		t.Fatalf("join room: expected 200, got %d", joinW.Code)
	}

	// 4. GET room again — same room, same latest game
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/rooms/"+code, nil)
	getReq2 = getReq2.WithContext(context.WithValue(getReq2.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
	}))
	getW2 := httptest.NewRecorder()
	roomHandler.GetRoom(getW2, getReq2)
	if getW2.Code != http.StatusOK {
		t.Fatalf("get room again: expected 200, got %d", getW2.Code)
	}
	var getResp2 store.GetRoomResponse
	if err := json.NewDecoder(getW2.Body).Decode(&getResp2); err != nil {
		t.Fatalf("decode get room 2: %v", err)
	}
	firstGameID := getResp2.LatestGame.ID

	// 5. Host creates new game (POST /api/rooms/{code}/games)
	gameBody, _ := json.Marshal(map[string]interface{}{})
	gameReq := httptest.NewRequest(http.MethodPost, "/api/rooms/"+code+"/games", bytes.NewReader(gameBody))
	gameReq.Header.Set("Content-Type", "application/json")
	gameReq = requestWithCodeChi(gameReq, code)
	gameReq = requestWithUserIDGame(gameReq, hostUser.ID)
	gameW := httptest.NewRecorder()
	gameHandler.CreateGame(gameW, gameReq)
	if gameW.Code != http.StatusCreated {
		t.Fatalf("create game: expected 201, got %d body=%s", gameW.Code, gameW.Body.String())
	}
	var createGameResp store.CreateGameResponse
	if err := json.NewDecoder(gameW.Body).Decode(&createGameResp); err != nil {
		t.Fatalf("decode create game: %v", err)
	}
	newGameID := createGameResp.Game.ID
	if newGameID == firstGameID {
		t.Error("expected new game to have different ID than initial game")
	}

	// 6. GET room returns new game as latest
	getReq3 := httptest.NewRequest(http.MethodGet, "/api/rooms/"+code, nil)
	getReq3 = getReq3.WithContext(context.WithValue(getReq3.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"code"}, Values: []string{code}},
	}))
	getW3 := httptest.NewRecorder()
	roomHandler.GetRoom(getW3, getReq3)
	if getW3.Code != http.StatusOK {
		t.Fatalf("get room after create game: expected 200, got %d", getW3.Code)
	}
	var getResp3 store.GetRoomResponse
	if err := json.NewDecoder(getW3.Body).Decode(&getResp3); err != nil {
		t.Fatalf("decode get room 3: %v", err)
	}
	if getResp3.LatestGame == nil {
		t.Fatal("expected latest_game after host created new game")
	}
	if getResp3.LatestGame.ID != newGameID {
		t.Errorf("expected latest_game.ID %s, got %s", newGameID, getResp3.LatestGame.ID)
	}
	if getResp3.LatestGameStateSnapshot == nil {
		t.Error("expected latest_game_state_snapshot")
	}
}
