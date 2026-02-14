package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/vntrieu/avalon/internal/auth"
	"github.com/vntrieu/avalon/internal/store"
)

// StartGameRequest is the body for POST /api/rooms/{code}/games.
// RoomPlayerID is required if no valid Authorization token is provided.
type StartGameRequest struct {
	RoomPlayerID string                 `json:"room_player_id,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
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
// @Description  Create a new game in the room. Only the room host may call this. Use Bearer token (from create/join room) or room_player_id in body.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        code  path      string               true   "Room code (6 alphanumeric)"
// @Param        body  body      StartGameRequest     false  "Request body (room_player_id required if no Bearer token)"
// @Success      201   {object}  store.CreateGameResponse
// @Failure      400   {string}  string  "Bad request or room has no players"
// @Failure      401   {string}  string  "Unauthorized (token or room_player_id required, or player not in room)"
// @Failure      403   {string}  string  "Only host can start a new game"
// @Failure      404   {string}  string  "Room not found"
// @Failure      500   {string}  string  "Server error"
// @Security     BearerAuth
// @Router       /api/rooms/{code}/games [post]
func (h *GameHandler) CreateGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	// Resolve room_player_id: from Bearer token or body
	roomPlayerID := body.RoomPlayerID
	if roomPlayerID == "" && len(h.tokenSecret) > 0 {
		if bearer := r.Header.Get("Authorization"); bearer != "" {
			const prefix = "Bearer "
			if strings.HasPrefix(bearer, prefix) {
				token := strings.TrimSpace(bearer[len(prefix):])
				claims, err := auth.VerifyToken(token, h.tokenSecret)
				if err == nil && claims.RoomPlayerID != "" {
					roomPlayerID = claims.RoomPlayerID
				}
			}
		}
	}
	if roomPlayerID == "" {
		http.Error(w, "unauthorized: room_player_id or valid token required", http.StatusUnauthorized)
		return
	}

	// Verify player is in room and is host
	player, err := h.roomStore.GetRoomPlayerInRoom(r.Context(), code, roomPlayerID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "room not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "player not in room") || strings.Contains(errMsg, "invalid room_player_id") {
			http.Error(w, "unauthorized: player not in room", http.StatusUnauthorized)
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
