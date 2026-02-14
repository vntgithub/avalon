package games

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/vntrieu/avalon/internal/store"
)

// ApplyMoveResult is returned by ApplyMove: new state, events to broadcast, and optional error.
type ApplyMoveResult struct {
	State  *GameState
	Events []BroadcastEvent
	Error  error
}

// BroadcastEvent represents an event to broadcast (type + payload).
type BroadcastEvent struct {
	Event   string                 `json:"event"`
	Payload map[string]interface{} `json:"payload"`
}

// GameStore interface for persistence (avoid circular import; implemented by store.GameStore + GameEventStore).
type GameStore interface {
	GetLatestSnapshot(ctx context.Context, gameID string) (map[string]interface{}, error)
	CreateOrUpdateSnapshot(ctx context.Context, gameID string, stateJSON map[string]interface{}) (int32, error)
	UpdateGameStatus(ctx context.Context, gameID string, status string, endedAt *time.Time) error
	GetGamePlayerIDsInOrder(ctx context.Context, gameID string) ([]string, error)
}

// GameEventStore interface for appending events.
type GameEventStore interface {
	CreateGameEvent(ctx context.Context, req store.CreateGameEventRequest) (*store.GameEvent, error)
}

// Engine applies moves and drives phase transitions.
type Engine struct {
	store   GameStore
	events  GameEventStore
	config  RulesConfig
}

// NewEngine creates an engine with the given stores and config.
func NewEngine(store GameStore, events GameEventStore, config RulesConfig) *Engine {
	if config.Phases == nil {
		config = ClassicAvalonConfig()
	}
	if config.TeamSizes == nil {
		config.TeamSizes = []int{2, 3, 2, 3, 3}
	}
	if config.FailThreshold <= 0 {
		config.FailThreshold = 3
	}
	return &Engine{store: store, events: events, config: config}
}

// GetState loads the latest snapshot for the game and returns a GameState. If no snapshot, returns nil.
func (e *Engine) GetState(ctx context.Context, gameID string) (*GameState, error) {
	m, err := e.store.GetLatestSnapshot(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	return StateFromMap(m), nil
}

// ApplyMove validates the move, applies it, persists event + snapshot, updates game status if finished.
// moveType is "vote" or "action"; for "action", payload should contain "action": "<action_type>" and action-specific fields.
func (e *Engine) ApplyMove(ctx context.Context, gameID string, roomPlayerID string, moveType string, payload map[string]interface{}) ApplyMoveResult {
	state, err := e.GetState(ctx, gameID)
	if err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("get state: %w", err)}
	}
	// No snapshot or lobby without players: only allow start_game (bootstrap from DB players).
	if state == nil || (state.Phase == PhaseLobby && len(state.PlayerIDs) == 0) {
		if moveType != "action" {
			return ApplyMoveResult{Error: fmt.Errorf("game not started; use action start_game")}
		}
		action, _ := payload["action"].(string)
		if action != ActionStartGame {
			return ApplyMoveResult{Error: fmt.Errorf("only start_game allowed in lobby")}
		}
		return e.bootstrapAndStart(ctx, gameID, roomPlayerID, payload)
	}

	if state.Status == "finished" {
		return ApplyMoveResult{Error: fmt.Errorf("game already finished")}
	}

	var next *GameState
	var events []BroadcastEvent

	switch moveType {
	case "vote":
		next, events, err = e.applyVote(ctx, state, roomPlayerID, payload)
	case "action":
		next, events, err = e.applyAction(ctx, state, roomPlayerID, payload)
	default:
		return ApplyMoveResult{Error: fmt.Errorf("unknown move type %q", moveType)}
	}
	if err != nil {
		return ApplyMoveResult{Error: err}
	}
	if next == nil {
		return ApplyMoveResult{Error: fmt.Errorf("no state update")}
	}

	// Persist: append event, write snapshot, update game status if finished
	eventPayload := payload
	if eventPayload == nil {
		eventPayload = make(map[string]interface{})
	}
	eventPayload["move_type"] = moveType
	_, err = e.events.CreateGameEvent(ctx, store.CreateGameEventRequest{
		GameID:       gameID,
		RoomPlayerID: &roomPlayerID,
		Type:         moveType,
		Payload:      eventPayload,
	})
	if err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("persist event: %w", err)}
	}

	stateMap := next.ToMap()
	version, err := e.store.CreateOrUpdateSnapshot(ctx, gameID, stateMap)
	if err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("persist snapshot: %w", err)}
	}
	next.Version = int(version)

	if next.Status == "finished" {
		now := time.Now()
		_ = e.store.UpdateGameStatus(ctx, gameID, "finished", &now)
	}

	return ApplyMoveResult{State: next, Events: events}
}

// bootstrapAndStart builds initial state from DB (player list) and transitions to team_selection.
func (e *Engine) bootstrapAndStart(ctx context.Context, gameID string, roomPlayerID string, payload map[string]interface{}) ApplyMoveResult {
	playerIDs, err := e.store.GetGamePlayerIDsInOrder(ctx, gameID)
	if err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("get players: %w", err)}
	}
	n := len(playerIDs)
	if n < e.config.MinPlayers || n > e.config.MaxPlayers {
		return ApplyMoveResult{Error: fmt.Errorf("player count %d not in range [%d,%d]", n, e.config.MinPlayers, e.config.MaxPlayers)}
	}

	// Assign simple roles: 2 evils for 5–6, 3 for 7+ (classic).
	roles := make(map[string]string)
	evilCount := 2
	if n >= 7 {
		evilCount = 3
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	order := rng.Perm(n)
	for i := 0; i < evilCount; i++ {
		roles[playerIDs[order[i]]] = "evil"
	}
	for _, id := range playerIDs {
		if roles[id] == "" {
			roles[id] = "good"
		}
	}

	teamSizes := e.config.TeamSizes
	if len(teamSizes) == 0 {
		teamSizes = DefaultTeamSizesForPlayerCount(n)
	}

	state := &GameState{
		GameID:       gameID,
		Phase:        PhaseTeamSelection,
		Status:       "in_progress",
		RoundIndex:   1,
		LeaderIndex:  0,
		PlayerIDs:    playerIDs,
		Roles:        roles,
		MissionResults: []string{},
	}
	stateMap := state.ToMap()
	stateMap["team_sizes"] = teamSizes
	version, err := e.store.CreateOrUpdateSnapshot(ctx, gameID, stateMap)
	if err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("create initial snapshot: %w", err)}
	}
	state.Version = int(version)
	if err := e.store.UpdateGameStatus(ctx, gameID, "in_progress", nil); err != nil {
		return ApplyMoveResult{Error: fmt.Errorf("update game status: %w", err)}
	}

	ev := BroadcastEvent{Event: "game_started", Payload: map[string]interface{}{
		"phase": state.Phase, "round_index": state.RoundIndex, "leader_id": state.LeaderPlayerID(),
	}}
	return ApplyMoveResult{State: state, Events: []BroadcastEvent{ev}}
}

func (e *Engine) applyVote(ctx context.Context, state *GameState, roomPlayerID string, payload map[string]interface{}) (*GameState, []BroadcastEvent, error) {
	if !e.isPlayerInGame(state, roomPlayerID) {
		return nil, nil, fmt.Errorf("player not in game")
	}

	switch state.Phase {
	case PhaseTeamVote:
		approved, ok := payload["approved"].(bool)
		if !ok {
			if s, ok := payload["approved"].(string); ok && (s == "true" || s == "false") {
				approved = s == "true"
			} else {
				return nil, nil, fmt.Errorf("payload must include approved: true/false")
			}
		}
		next := state.Clone()
		if next.TeamVotes == nil {
			next.TeamVotes = make(map[string]string)
		}
		if _, exists := next.TeamVotes[roomPlayerID]; exists {
			return nil, nil, fmt.Errorf("already voted")
		}
		v := "reject"
		if approved {
			v = "approve"
		}
		next.TeamVotes[roomPlayerID] = v
		// Check if all voted
		if len(next.TeamVotes) >= len(next.PlayerIDs) {
			approveCount := 0
			for _, v := range next.TeamVotes {
				if v == "approve" {
					approveCount++
				}
			}
			if approveCount > len(next.PlayerIDs)/2 {
				// Team approved -> mission_vote
				next.Phase = PhaseMissionVote
				next.TeamVotes = nil
				ev := BroadcastEvent{Event: "team_approved", Payload: map[string]interface{}{"phase": next.Phase}}
				return next, []BroadcastEvent{ev}, nil
			}
			// Rejected -> next leader, back to team_selection
				next.RejectCount++
				next.Phase = PhaseTeamSelection
				next.LeaderIndex = (next.LeaderIndex + 1) % len(next.PlayerIDs)
				next.ProposedTeam = nil
				next.TeamVotes = nil
				ev := BroadcastEvent{Event: "team_rejected", Payload: map[string]interface{}{
					"phase": next.Phase, "reject_count": next.RejectCount, "leader_id": next.LeaderPlayerID()}}
				return next, []BroadcastEvent{ev}, nil
		}
		return next, []BroadcastEvent{{Event: "vote_recorded", Payload: map[string]interface{}{"player_id": roomPlayerID}}}, nil

	case PhaseMissionVote:
		success, ok := payload["success"].(bool)
		if !ok {
			if s, ok := payload["success"].(string); ok && (s == "true" || s == "false") {
				success = s == "true"
			} else {
				return nil, nil, fmt.Errorf("payload must include success: true/false for mission vote")
			}
		}
		if !e.isOnProposedTeam(state, roomPlayerID) {
			return nil, nil, fmt.Errorf("only team members can submit mission vote")
		}
		next := state.Clone()
		if next.MissionVotes == nil {
			next.MissionVotes = make(map[string]string)
		}
		if _, exists := next.MissionVotes[roomPlayerID]; exists {
			return nil, nil, fmt.Errorf("already voted")
		}
		if success {
			next.MissionVotes[roomPlayerID] = "success"
		} else {
			next.MissionVotes[roomPlayerID] = "fail"
		}
		teamSize := len(state.ProposedTeam)
		if len(next.MissionVotes) >= teamSize {
			// Resolution: any fail -> mission fail
			failCount := 0
			for _, v := range next.MissionVotes {
				if v == "fail" {
					failCount++
				}
			}
			result := "success"
			if failCount > 0 {
				result = "fail"
			}
			next.MissionResults = append(next.MissionResults, result)
			next.MissionVotes = nil
			next.ProposedTeam = nil
			next.Phase = PhaseMissionResolution
			// Transition: next round or game end
			next.Phase = PhaseTeamSelection
			next.LeaderIndex = (next.LeaderIndex + 1) % len(next.PlayerIDs)
			next.RejectCount = 0
			next.RoundIndex++
			failTotal := 0
			for _, r := range next.MissionResults {
				if r == "fail" {
					failTotal++
				}
			}
			successTotal := len(next.MissionResults) - failTotal
			if failTotal >= e.config.FailThreshold {
				next.Status = "finished"
				next.Phase = PhaseFinished
				next.Winner = "evil"
				ev := BroadcastEvent{Event: "game_ended", Payload: map[string]interface{}{"winner": next.Winner, "mission_result": result}}
				return next, []BroadcastEvent{ev}, nil
			}
			if successTotal >= 3 {
				next.Status = "finished"
				next.Phase = PhaseFinished
				next.Winner = "good"
				ev := BroadcastEvent{Event: "game_ended", Payload: map[string]interface{}{"winner": next.Winner, "mission_result": result}}
				return next, []BroadcastEvent{ev}, nil
			}
			ev := BroadcastEvent{Event: "mission_resolved", Payload: map[string]interface{}{
				"result": result, "round_index": next.RoundIndex, "leader_id": next.LeaderPlayerID(), "phase": next.Phase}}
			return next, []BroadcastEvent{ev}, nil
		}
		return next, []BroadcastEvent{{Event: "vote_recorded", Payload: map[string]interface{}{"player_id": roomPlayerID}}}, nil
	}

	return nil, nil, fmt.Errorf("vote not allowed in phase %s", state.Phase)
}

func (e *Engine) applyAction(ctx context.Context, state *GameState, roomPlayerID string, payload map[string]interface{}) (*GameState, []BroadcastEvent, error) {
	action, _ := payload["action"].(string)
	if action == "" {
		action, _ = payload["type"].(string)
	}
	if action == "" {
		return nil, nil, fmt.Errorf("payload must include action or type")
	}

	allowed := e.getAllowedActions(state.Phase)
	ok := false
	for _, a := range allowed {
		if a == action {
			ok = true
			break
		}
	}
	if !ok {
		return nil, nil, fmt.Errorf("action %q not allowed in phase %s", action, state.Phase)
	}

	switch action {
	case ActionStartGame:
		// Handled in bootstrapAndStart when state is nil
		return nil, nil, fmt.Errorf("game already started")
	case ActionProposeTeam:
		if state.LeaderPlayerID() != roomPlayerID {
			return nil, nil, fmt.Errorf("only the leader can propose a team")
		}
		team, ok := stringSliceFromPayload(payload["team_ids"])
		if !ok {
			team, ok = stringSliceFromPayload(payload["team"])
			if !ok {
				return nil, nil, fmt.Errorf("payload must include team_ids or team (array of room_player_id)")
			}
		}
		teamSizes := e.config.TeamSizes
		if len(teamSizes) == 0 {
			teamSizes = DefaultTeamSizesForPlayerCount(len(state.PlayerIDs))
		}
		roundIdx := state.RoundIndex
		if roundIdx <= 0 || roundIdx > len(teamSizes) {
			roundIdx = 1
		}
		requiredSize := teamSizes[roundIdx-1]
		if len(team) != requiredSize {
			return nil, nil, fmt.Errorf("team must have exactly %d members for this round", requiredSize)
		}
		for _, id := range team {
			if !e.isPlayerInGame(state, id) {
				return nil, nil, fmt.Errorf("team includes non-player %s", id)
			}
		}
		next := state.Clone()
		next.ProposedTeam = team
		next.Phase = PhaseTeamVote
		next.TeamVotes = make(map[string]string)
		ev := BroadcastEvent{Event: "team_proposed", Payload: map[string]interface{}{"team": team, "phase": next.Phase}}
		return next, []BroadcastEvent{ev}, nil
	}

	return nil, nil, fmt.Errorf("action %q not implemented", action)
}

func (e *Engine) getAllowedActions(phase string) []string {
	for _, p := range e.config.Phases {
		if p.Name == phase {
			return p.AllowedActions
		}
	}
	return nil
}

func (e *Engine) isPlayerInGame(state *GameState, roomPlayerID string) bool {
	for _, id := range state.PlayerIDs {
		if id == roomPlayerID {
			return true
		}
	}
	return false
}

func (e *Engine) isOnProposedTeam(state *GameState, roomPlayerID string) bool {
	for _, id := range state.ProposedTeam {
		if id == roomPlayerID {
			return true
		}
	}
	return false
}

func stringSliceFromPayload(v interface{}) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// LoadConfigFromMap loads RulesConfig from game config_json (e.g. from DB). Falls back to ClassicAvalonConfig.
func LoadConfigFromMap(configJSON map[string]interface{}) RulesConfig {
	if configJSON == nil {
		return ClassicAvalonConfig()
	}
	// Optional: parse preset name or full phases from config
	if preset, ok := configJSON["preset"].(string); ok && preset == "classic" {
		return ClassicAvalonConfig()
	}
	return ClassicAvalonConfig()
}

// StateToMapForSync serializes state for sync_state response (same as ToMap but ensures JSON-safe).
func StateToMapForSync(state *GameState) (map[string]interface{}, error) {
	if state == nil {
		return map[string]interface{}{}, nil
	}
	m := state.ToMap()
	// Remove or mask roles if not revealed per rules
	return m, nil
}

// DecodePayload ensures payload is map[string]interface{} (from JSON).
func DecodePayload(raw interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}
	if m, ok := raw.(map[string]interface{}); ok {
		return m
	}
	if b, ok := raw.([]byte); ok {
		var m map[string]interface{}
		if json.Unmarshal(b, &m) == nil {
			return m
		}
	}
	return nil
}
