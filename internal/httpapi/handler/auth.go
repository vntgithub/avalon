package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/vntrieu/avalon/internal/auth"
	"github.com/vntrieu/avalon/internal/store"
)

// Auth validation limits.
const (
	EmailMaxLen       = 256
	PasswordMinLen    = 8
	PasswordMaxLenAuth = 128
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// RegisterRequest is the body for POST /api/auth/register.
type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// LoginRequest is the body for POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse is the response for register and login (user + token).
type AuthResponse struct {
	User      *store.User `json:"user"`
	Token     string      `json:"token"`
	ExpiresAt string      `json:"expires_at"`
}

// AuthHandler handles auth and user endpoints.
type AuthHandler struct {
	userStore   *store.UserStore
	tokenSecret []byte
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userStore *store.UserStore, tokenSecret []byte) *AuthHandler {
	return &AuthHandler{userStore: userStore, tokenSecret: tokenSecret}
}

func validateEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "email is required"
	}
	if len(email) > EmailMaxLen {
		return "email must be at most 256 characters"
	}
	if !emailRegex.MatchString(email) {
		return "invalid email format"
	}
	return ""
}

func validatePasswordAuth(password string) string {
	if len(password) < PasswordMinLen {
		return "password must be at least 8 characters"
	}
	if len(password) > PasswordMaxLenAuth {
		return "password must be at most 128 characters"
	}
	return ""
}

// Register handles POST /api/auth/register
//
// @Summary      Register
// @Description  Create a new user account. Returns user and session token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  RegisterRequest  true  "Request body"
// @Success      201   {object}  AuthResponse
// @Failure      400   {string}  string  "Bad request (validation)"
// @Failure      409   {string}  string  "Email already registered"
// @Failure      500   {string}  string  "Server error"
// @Router       /api/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if msg := validateEmail(req.Email); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if msg := validatePasswordAuth(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if msg := validateDisplayName(req.DisplayName); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	user, err := h.userStore.CreateUser(r.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if err == store.ErrEmailExists {
			http.Error(w, "email already registered", http.StatusConflict)
			return
		}
		log.Printf("[%s] register error: %v", requestID(r), err)
		http.Error(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	token, expiresAt, err := auth.GenerateUserToken(user.ID, h.tokenSecret, auth.DefaultUserTokenExpiry)
	if err != nil {
		log.Printf("[%s] generate user token error: %v", requestID(r), err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(AuthResponse{
		User:      user,
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// Login handles POST /api/auth/login
//
// @Summary      Login
// @Description  Authenticate with email and password. Returns user and session token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  LoginRequest  true  "Request body"
// @Success      200   {object}  AuthResponse
// @Failure      400   {string}  string  "Bad request"
// @Failure      401   {string}  string  "Invalid email or password"
// @Failure      500   {string}  string  "Server error"
// @Router       /api/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if msg := validateEmail(req.Email); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.Password == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	user, err := h.userStore.VerifyPassword(r.Context(), req.Email, req.Password)
	if err != nil {
		log.Printf("[%s] login verify error: %v", requestID(r), err)
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}
	if user == nil {
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	token, expiresAt, err := auth.GenerateUserToken(user.ID, h.tokenSecret, auth.DefaultUserTokenExpiry)
	if err != nil {
		log.Printf("[%s] generate user token error: %v", requestID(r), err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(AuthResponse{
		User:      user,
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// GetMe handles GET /api/users/me
//
// @Summary      Get current user
// @Description  Return the authenticated user's profile. Requires Bearer token.
// @Tags         users
// @Produce      json
// @Success      200   {object}  store.User
// @Failure      401   {string}  string  "Unauthorized"
// @Router       /api/users/me [get]
// @Security     BearerAuth
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := UserIDFromRequest(r)
	if userID == nil || *userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.userStore.GetUserByID(r.Context(), *userID)
	if err != nil {
		log.Printf("[%s] get user error: %v", requestID(r), err)
		http.Error(w, "failed to get user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
}
