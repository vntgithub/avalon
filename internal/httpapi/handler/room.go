package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vntrieu/avalon/internal/auth"
	"github.com/vntrieu/avalon/internal/store"
)

// Validation limits for room endpoints.
const (
	DisplayNameMinLen = 1
	DisplayNameMaxLen = 64
	PasswordMaxLen    = 128
)

// roomCodePattern matches 6-char alphanumeric codes (same charset as generateRoomCode: A-Z excluding I,O; 2-9).
var roomCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{6}$`)

// RoomHandler handles room-related HTTP requests.
type RoomHandler struct {
	roomStore   *store.RoomStore
	tokenSecret []byte
}

// NewRoomHandler creates a new RoomHandler. If tokenSecret is non-empty, create/join responses include a WebSocket auth token.
func NewRoomHandler(roomStore *store.RoomStore, tokenSecret []byte) *RoomHandler {
	return &RoomHandler{roomStore: roomStore, tokenSecret: tokenSecret}
}

func validateDisplayName(displayName string) string {
	s := strings.TrimSpace(displayName)
	if len(s) < DisplayNameMinLen {
		return "display_name is required"
	}
	if len(s) > DisplayNameMaxLen {
		return fmt.Sprintf("display_name must be at most %d characters", DisplayNameMaxLen)
	}
	return ""
}

func validatePasswordLength(password string) string {
	if len(password) > PasswordMaxLen {
		return "password must be at most 128 characters"
	}
	return ""
}

func validateRoomCode(code string) bool {
	return len(code) == 6 && roomCodePattern.MatchString(code)
}

// CreateRoom handles POST /api/rooms
//
// @Summary      Create room
// @Description  Create a new room. The requester becomes the host.
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        body  body      store.CreateRoomRequest   true  "Request body"
// @Success      201   {object}  store.CreateRoomResponse
// @Failure      400   {string}  string  "Bad request (invalid display_name, password length, or body)"
// @Failure      500   {string}  string  "Server error"
// @Router       /api/rooms [post]
func (h *RoomHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req store.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if msg := validateDisplayName(req.DisplayName); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if msg := validatePasswordLength(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	resp, err := h.roomStore.CreateRoom(r.Context(), req)
	if err != nil {
		log.Printf("[%s] create room error: %v", requestID(r), err)
		http.Error(w, "failed to create room", http.StatusInternalServerError)
		return
	}

	if len(h.tokenSecret) > 0 {
		token, expiresAt, err := auth.GenerateToken(resp.Room.ID, resp.RoomPlayer.ID, h.tokenSecret, auth.DefaultTokenExpiry)
		if err != nil {
			log.Printf("[%s] generate token error: %v", requestID(r), err)
			http.Error(w, "failed to create room", http.StatusInternalServerError)
			return
		}
		resp.Token = token
		resp.ExpiresAt = &expiresAt
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[%s] encode response error: %v", requestID(r), err)
	}
}

// JoinRoom handles POST /api/rooms/{code}/join
//
// @Summary      Join room
// @Description  Join an existing room. Returns room, player, and optional latest game/snapshot.
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        code  path      string                    true   "Room code (6 alphanumeric)"
// @Param        body  body      store.JoinRoomRequest     true   "Request body (code in path, not body)"
// @Success      200   {object}  store.JoinRoomResponse
// @Failure      400   {string}  string  "Bad request"
// @Failure      401   {string}  string  "Password required or invalid"
// @Failure      404   {string}  string  "Room not found"
// @Failure      409   {string}  string  "Display name already taken in this room"
// @Failure      500   {string}  string  "Server error"
// @Router       /api/rooms/{code}/join [post]
func (h *RoomHandler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, "room code is required", http.StatusBadRequest)
		return
	}
	if !validateRoomCode(code) {
		http.Error(w, "invalid room code format", http.StatusBadRequest)
		return
	}

	var req store.JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Code = code

	if msg := validateDisplayName(req.DisplayName); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if msg := validatePasswordLength(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	resp, err := h.roomStore.JoinRoom(r.Context(), req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "room not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		if errMsg == "password is required" {
			http.Error(w, errMsg, http.StatusUnauthorized)
			return
		}
		if errMsg == "invalid password" {
			http.Error(w, errMsg, http.StatusUnauthorized)
			return
		}
		if errMsg == "display name already taken in this room" {
			http.Error(w, errMsg, http.StatusConflict)
			return
		}
		log.Printf("[%s] join room error: %v", requestID(r), err)
		http.Error(w, "failed to join room", http.StatusInternalServerError)
		return
	}

	if len(h.tokenSecret) > 0 {
		token, expiresAt, err := auth.GenerateToken(resp.Room.ID, resp.RoomPlayer.ID, h.tokenSecret, auth.DefaultTokenExpiry)
		if err != nil {
			log.Printf("[%s] generate token error: %v", requestID(r), err)
			http.Error(w, "failed to join room", http.StatusInternalServerError)
			return
		}
		resp.Token = token
		resp.ExpiresAt = &expiresAt
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[%s] encode response error: %v", requestID(r), err)
	}
}

// GetRoom handles GET /api/rooms/{code}
//
// @Summary      Get room
// @Description  Get room details and latest game state. No authentication required.
// @Tags         rooms
// @Produce      json
// @Param        code  path      string  true  "Room code (6 alphanumeric)"
// @Success      200   {object}  store.GetRoomResponse
// @Failure      400   {string}  string  "Invalid room code"
// @Failure      404   {string}  string  "Room not found"
// @Failure      500   {string}  string  "Server error"
// @Router       /api/rooms/{code} [get]
func (h *RoomHandler) GetRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, "room code is required", http.StatusBadRequest)
		return
	}
	if !validateRoomCode(code) {
		http.Error(w, "invalid room code format", http.StatusBadRequest)
		return
	}

	resp, err := h.roomStore.GetRoom(r.Context(), code)
	if err != nil {
		if err.Error() == "room not found" {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		log.Printf("[%s] get room error: %v", requestID(r), err)
		http.Error(w, "failed to get room", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[%s] encode response error: %v", requestID(r), err)
	}
}
