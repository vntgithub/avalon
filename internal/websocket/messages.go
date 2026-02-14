package websocket

import "github.com/vntrieu/avalon/internal/store"

// OutgoingMessage is what the hub sends to clients; exactly one of GameEvent or Envelope is set.
type OutgoingMessage struct {
	GameEvent *store.GameEvent  // for game WS
	Envelope  *ServerEnvelope   // for room WS
}

// ClientInMessage is the envelope for messages from client to server.
// Types: "chat" | "vote" | "action" | "system"
type ClientInMessage struct {
	Type           string                 `json:"type"`
	CorrelationID  string                 `json:"correlation_id,omitempty"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
}

// ServerEnvelope is the envelope for messages from server to client.
// Type: "event" | "state" | "error"
type ServerEnvelope struct {
	Type    string                 `json:"type"`
	Event   string                 `json:"event,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// Chat payload from client (type: "chat").
const ClientMessageTypeChat = "chat"

// Client message types for game flow.
const (
	ClientMessageTypeVote   = "vote"
	ClientMessageTypeAction = "action"
	ClientMessageTypeSyncState = "sync_state"
)

// Server event types.
const (
	ServerEventChat          = "chat"
	ServerEventVoteRecorded  = "vote_recorded"
	ServerEventState         = "state"
	ServerEventGameEnded     = "game_ended"
	ServerEventTeamProposed  = "team_proposed"
	ServerEventTeamApproved  = "team_approved"
	ServerEventTeamRejected  = "team_rejected"
	ServerEventMissionResolved = "mission_resolved"
)

// Server envelope types.
const (
	ServerTypeEvent = "event"
	ServerTypeState = "state"
	ServerTypeError = "error"
)

// MaxChatMessageLength is the maximum allowed length for a chat message.
const MaxChatMessageLength = 2000

// MaxClientMessageTypeLength limits the "type" field to prevent abuse.
const MaxClientMessageTypeLength = 64

// ValidClientMessageTypes are the only allowed values for ClientInMessage.Type (room WS).
var ValidClientMessageTypes = map[string]bool{
	ClientMessageTypeChat:     true,
	ClientMessageTypeVote:     true,
	ClientMessageTypeAction:   true,
	ClientMessageTypeSyncState: true,
}
