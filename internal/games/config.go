package games

// PhaseDef defines a phase: name and allowed action types.
type PhaseDef struct {
	Name           string   `json:"name"`
	AllowedActions []string `json:"allowed_actions"`
}

// RulesConfig holds phase sequence and constraints (e.g. team size per round).
type RulesConfig struct {
	Phases    []PhaseDef `json:"phases"`
	MinPlayers int       `json:"min_players"`
	MaxPlayers int       `json:"max_players"`
	// TeamSizes per round (1-based round index). If nil, use default Avalon sizes.
	TeamSizes []int `json:"team_sizes,omitempty"`
	// FailThreshold: number of mission failures for evil to win (default 3).
	FailThreshold int `json:"fail_threshold,omitempty"`
}

// ClassicAvalonPhases defines the phase sequence for classic Avalon.
var ClassicAvalonPhases = []PhaseDef{
	{Name: PhaseLobby, AllowedActions: []string{ActionStartGame}},
	{Name: PhaseTeamSelection, AllowedActions: []string{ActionProposeTeam}},
	{Name: PhaseTeamVote, AllowedActions: []string{ActionVote}},
	{Name: PhaseMissionVote, AllowedActions: []string{ActionVote}},
	{Name: PhaseMissionResolution, AllowedActions: []string{}}, // system only
	{Name: PhaseFinished, AllowedActions: []string{}},
}

// Phase names.
const (
	PhaseLobby             = "lobby"
	PhaseTeamSelection     = "team_selection"
	PhaseTeamVote          = "team_vote"
	PhaseMissionVote       = "mission_vote"
	PhaseMissionResolution = "mission_resolution"
	PhaseFinished          = "finished"
)

// Action types.
const (
	ActionStartGame    = "start_game"
	ActionProposeTeam  = "propose_team"
	ActionVote         = "vote"
	ActionMissionVote  = "vote" // same type, different phase
)

// DefaultTeamSizesForPlayerCount returns mission team sizes for 5–10 players (classic Avalon).
func DefaultTeamSizesForPlayerCount(n int) []int {
	switch n {
	case 5:
		return []int{2, 3, 2, 3, 3}
	case 6:
		return []int{2, 3, 4, 3, 4}
	case 7:
		return []int{2, 3, 3, 4, 4}
	case 8:
		return []int{3, 4, 4, 5, 5}
	case 9:
		return []int{3, 4, 4, 5, 5}
	case 10:
		return []int{3, 4, 4, 5, 5}
	default:
		// fallback for 5
		return []int{2, 3, 2, 3, 3}
	}
}

// ClassicAvalonConfig returns a RulesConfig for classic Avalon.
func ClassicAvalonConfig() RulesConfig {
	return RulesConfig{
		Phases:         ClassicAvalonPhases,
		MinPlayers:     5,
		MaxPlayers:     10,
		FailThreshold:  3,
	}
}
