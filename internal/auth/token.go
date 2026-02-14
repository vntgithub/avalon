package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims holds room and player identity for WebSocket auth.
type Claims struct {
	RoomID      string `json:"room_id"`
	RoomPlayerID string `json:"room_player_id"`
	Exp         int64  `json:"exp"`
}

// DefaultTokenExpiry is the default lifetime for WebSocket auth tokens.
const DefaultTokenExpiry = 24 * time.Hour

// GenerateToken creates an HMAC-SHA256 signed token with room_id, room_player_id, and expiry.
// Format: base64url(payload).base64url(signature).
func GenerateToken(roomID, roomPlayerID string, secret []byte, expiry time.Duration) (token string, expiresAt time.Time, err error) {
	if len(secret) == 0 {
		return "", time.Time{}, fmt.Errorf("token secret is required")
	}
	expiresAt = time.Now().UTC().Add(expiry)
	claims := Claims{
		RoomID:       roomID,
		RoomPlayerID: roomPlayerID,
		Exp:          expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal claims: %w", err)
	}
	b64Payload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(b64Payload))
	sig := mac.Sum(nil)
	b64Sig := base64.RawURLEncoding.EncodeToString(sig)
	return b64Payload + "." + b64Sig, expiresAt, nil
}

// VerifyToken verifies the signature and returns claims. Returns error if expired or invalid.
func VerifyToken(token string, secret []byte) (*Claims, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("token secret is required")
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token format")
	}
	b64Payload, b64Sig := parts[0], parts[1]

	// Verify signature
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(b64Payload))
	expectedSig := mac.Sum(nil)
	sig, err := base64.RawURLEncoding.DecodeString(b64Sig)
	if err != nil {
		return nil, fmt.Errorf("invalid token signature encoding: %w", err)
	}
	if !hmac.Equal(sig, expectedSig) {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid token payload encoding: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid token payload: %w", err)
	}

	// Check expiry
	if time.Now().UTC().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}
	if claims.RoomID == "" || claims.RoomPlayerID == "" {
		return nil, fmt.Errorf("invalid token claims: missing room_id or room_player_id")
	}
	return &claims, nil
}
