package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/vntrieu/avalon/internal/store"
)

// StartGameRequest is the body for POST /api/rooms/{code}/games.
// Requires user token; room player is resolved from the authenticated user.
type StartGameRequest struct {
	Config map[string]interface{} `json:"config,omitempty"`
}

// GameHandler handles game-related HTTP requests.
type GameHandler struct {
	gameStore   *store.GameStore
	roomStore   *store.RoomStore
	tokenSecret []byte
}

// NewGameHandler creates a new GameHandler. tokenSecret is used to verify Bearer tokens for host auth.
func NewGameHandler(gameStore *store.GameStore, roomStore *store.RoomStore, tokenSecret []byte) *GameHandler {
	return &GameHandler{gameStore: gameStore, roomStore: roomStore, tokenSecret: tokenSecret}
}

// CreateGame handles POST /api/rooms/{code}/games (host only; creates a new game and initial snapshot).
//
// @Summary      Create game
// @Description  Create a new game in the room. Requires user token; only the room host may call this. Room player is resolved from the authenticated user.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        code  path      string               true   "Room code (6 alphanumeric)"
// @Param        body  body      StartGameRequest     false  "Request body (config optional)"
// @Success      201   {object}  store.CreateGameResponse
// @Failure      400   {string}  string  "Bad request or room has no players"
// @Failure      401   {string}  string  "Unauthorized (user token required)"
// @Failure      403   {string}  string  "Only host can start a new game, or user not in room"
// @Failure      404   {string}  string  "Room not found"
// @Failure      500   {string}  string  "Server error"
// @Security     BearerAuth
// @Router       /api/rooms/{code}/games [post]
func (h *GameHandler) CreateGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := UserIDFromRequest(r)
	if userID == nil || *userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	var body StartGameRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Resolve room player from authenticated user
	player, err := h.roomStore.GetRoomPlayerByUserInRoom(r.Context(), code, *userID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "room not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "user not in room") {
			http.Error(w, "forbidden: you are not a player in this room", http.StatusForbidden)
			return
		}
		log.Printf("[%s] get room player error: %v", requestID(r), err)
		http.Error(w, "failed to verify player", http.StatusInternalServerError)
		return
	}
	if !player.IsHost {
		http.Error(w, "forbidden: only the host can start a new game", http.StatusForbidden)
		return
	}

	req := store.CreateGameRequest{Code: code, Config: body.Config}
	resp, err := h.gameStore.CreateGame(r.Context(), req)
	if err != nil {
		log.Printf("[%s] create game error: %v", requestID(r), err)
		errMsg := err.Error()
		if strings.Contains(errMsg, "room not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "room has no players") {
			http.Error(w, "cannot create game: room has no players", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to create game", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[%s] encode response error: %v", requestID(r), err)
	}
}
