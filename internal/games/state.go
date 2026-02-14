package games

// GameState is the full engine state, serialized to JSON for snapshots.
type GameState struct {
	GameID    string   `json:"game_id"`
	Phase     string   `json:"phase"`
	Status    string   `json:"status"` // waiting | in_progress | finished
	RoundIndex int     `json:"round_index"` // 1-based mission round
	LeaderIndex int    `json:"leader_index"` // index into PlayerIDs
	PlayerIDs  []string `json:"player_ids"`  // room_player_id in order (determines leader rotation)
	// Roles: map room_player_id -> role (e.g. "good", "evil", "merlin"). Omitted until game end or per rules.
	Roles map[string]string `json:"roles,omitempty"`
	// ProposedTeam is set during team_selection/team_vote (the current proposal).
	ProposedTeam []string `json:"proposed_team,omitempty"`
	// TeamVotes: for team_vote phase, map room_player_id -> "approve" | "reject"
	TeamVotes map[string]string `json:"team_votes,omitempty"`
	// MissionVotes: for mission_vote phase, map room_player_id -> "success" | "fail"
	MissionVotes map[string]string `json:"mission_votes,omitempty"`
	// MissionResults: per-round result "success" | "fail" (after resolution).
	MissionResults []string `json:"mission_results,omitempty"`
	// RejectCount: number of consecutive team rejections (resets when team approved).
	RejectCount int `json:"reject_count,omitempty"`
	// Winner: "good" | "evil" when status == finished.
	Winner string `json:"winner,omitempty"`
	// Version is incremented on each snapshot write (optional, can be set by store).
	Version int `json:"version,omitempty"`
}

// Clone returns a shallow copy of the state (safe for maps that we replace, not deep-clone).
func (s *GameState) Clone() *GameState {
	if s == nil {
		return nil
	}
	out := *s
	if s.Roles != nil {
		out.Roles = make(map[string]string, len(s.Roles))
		for k, v := range s.Roles {
			out.Roles[k] = v
		}
	}
	if s.ProposedTeam != nil {
		out.ProposedTeam = make([]string, len(s.ProposedTeam))
		copy(out.ProposedTeam, s.ProposedTeam)
	}
	if s.TeamVotes != nil {
		out.TeamVotes = make(map[string]string, len(s.TeamVotes))
		for k, v := range s.TeamVotes {
			out.TeamVotes[k] = v
		}
	}
	if s.MissionVotes != nil {
		out.MissionVotes = make(map[string]string, len(s.MissionVotes))
		for k, v := range s.MissionVotes {
			out.MissionVotes[k] = v
		}
	}
	if s.MissionResults != nil {
		out.MissionResults = make([]string, len(s.MissionResults))
		copy(out.MissionResults, s.MissionResults)
	}
	return &out
}

// LeaderPlayerID returns the room_player_id of the current leader, or "" if none.
func (s *GameState) LeaderPlayerID() string {
	if s == nil || s.LeaderIndex < 0 || s.LeaderIndex >= len(s.PlayerIDs) {
		return ""
	}
	return s.PlayerIDs[s.LeaderIndex]
}

// ToMap converts state to a map for JSON snapshot (engine uses this for persistence).
func (s *GameState) ToMap() map[string]interface{} {
	if s == nil {
		return nil
	}
	m := map[string]interface{}{
		"game_id":      s.GameID,
		"phase":        s.Phase,
		"status":       s.Status,
		"round_index":  s.RoundIndex,
		"leader_index": s.LeaderIndex,
		"player_ids":   s.PlayerIDs,
		"reject_count": s.RejectCount,
		"version":      s.Version,
	}
	if len(s.Roles) > 0 {
		m["roles"] = s.Roles
	}
	if len(s.ProposedTeam) > 0 {
		m["proposed_team"] = s.ProposedTeam
	}
	if len(s.TeamVotes) > 0 {
		m["team_votes"] = s.TeamVotes
	}
	if len(s.MissionVotes) > 0 {
		m["mission_votes"] = s.MissionVotes
	}
	if len(s.MissionResults) > 0 {
		m["mission_results"] = s.MissionResults
	}
	if s.Winner != "" {
		m["winner"] = s.Winner
	}
	return m
}

// StateFromMap reconstructs GameState from a snapshot map (e.g. from DB).
func StateFromMap(m map[string]interface{}) *GameState {
	if m == nil {
		return nil
	}
	s := &GameState{}
	if v, ok := m["game_id"].(string); ok {
		s.GameID = v
	}
	if v, ok := m["phase"].(string); ok {
		s.Phase = v
	}
	if v, ok := m["status"].(string); ok {
		s.Status = v
	}
	if v, ok := floatToInt(m["round_index"]); ok {
		s.RoundIndex = v
	}
	if v, ok := floatToInt(m["leader_index"]); ok {
		s.LeaderIndex = v
	}
	if v, ok := stringSlice(m["player_ids"]); ok {
		s.PlayerIDs = v
	}
	if v, ok := stringMap(m["roles"]); ok {
		s.Roles = v
	}
	if v, ok := stringSlice(m["proposed_team"]); ok {
		s.ProposedTeam = v
	}
	if v, ok := stringMap(m["team_votes"]); ok {
		s.TeamVotes = v
	}
	if v, ok := stringMap(m["mission_votes"]); ok {
		s.MissionVotes = v
	}
	if v, ok := stringSlice(m["mission_results"]); ok {
		s.MissionResults = v
	}
	if v, ok := floatToInt(m["reject_count"]); ok {
		s.RejectCount = v
	}
	if v, ok := m["winner"].(string); ok {
		s.Winner = v
	}
	if v, ok := floatToInt(m["version"]); ok {
		s.Version = v
	}
	return s
}

func floatToInt(a interface{}) (int, bool) {
	switch v := a.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func stringSlice(a interface{}) ([]string, bool) {
	switch v := a.(type) {
	case []string:
		return v, true
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	}
	return nil, false
}

func stringMap(a interface{}) (map[string]string, bool) {
	m, ok := a.(map[string]interface{})
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, true
}
