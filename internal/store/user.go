package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/vntrieu/avalon/internal/db"
)

// User represents a registered user (API response excludes password_hash).
type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ErrEmailExists is returned when registering with an email that is already in use.
var ErrEmailExists = errors.New("email already registered")

// UserStore handles database operations for users.
type UserStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewUserStore creates a new UserStore.
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{
		pool:    pool,
		queries: db.New(pool),
	}
}

// CreateUser creates a new user with hashed password. Returns error if email already exists.
func (s *UserStore) CreateUser(ctx context.Context, email, password, displayName string) (*User, error) {
	exists, err := s.queries.CheckUserEmailExists(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("check email exists: %w", err)
	}
	if exists {
		return nil, ErrEmailExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	params := db.CreateUserParams{
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		AvatarUrl:    pgtype.Text{Valid: false},
		SettingsJson: []byte("{}"),
	}
	row, err := s.queries.CreateUser(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return dbUserToStoreUser(&row), nil
}

// GetUserByEmail returns the user by email. Returns nil, error when not found.
func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return dbUserToStoreUser(&row), nil
}

// GetUserByID returns the user by id. Returns nil, error when not found.
func (s *UserStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	uid, err := stringToUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	row, err := s.queries.GetUserByID(ctx, uid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return dbUserToStoreUser(&row), nil
}

// VerifyPassword checks the password against the stored hash.
func (s *UserStore) VerifyPassword(ctx context.Context, email, password string) (*User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		return nil, nil
	}
	return dbUserToStoreUser(&row), nil
}

func dbUserToStoreUser(u *db.User) *User {
	out := &User{
		ID:          uuidToString(u.ID),
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   timestamptzToTime(u.CreatedAt),
		UpdatedAt:   timestamptzToTime(u.UpdatedAt),
	}
	if u.AvatarUrl.Valid {
		out.AvatarURL = &u.AvatarUrl.String
	}
	return out
}
