package games

import (
	"context"
	"testing"
	"time"

	"github.com/vntrieu/avalon/internal/store"
)

func TestStateFromMap_ToMap_RoundTrip(t *testing.T) {
	s := &GameState{
		GameID:       "game-1",
		Phase:        PhaseTeamSelection,
		Status:       "in_progress",
		RoundIndex:   1,
		LeaderIndex:  0,
		PlayerIDs:    []string{"p1", "p2", "p3"},
		RejectCount:  0,
	}
	m := s.ToMap()
	back := StateFromMap(m)
	if back == nil {
		t.Fatal("StateFromMap returned nil")
	}
	if back.GameID != s.GameID || back.Phase != s.Phase || back.RoundIndex != s.RoundIndex {
		t.Errorf("round trip mismatch: got %+v", back)
	}
	if len(back.PlayerIDs) != len(s.PlayerIDs) {
		t.Errorf("player_ids length: got %d want %d", len(back.PlayerIDs), len(s.PlayerIDs))
	}
}

func TestClassicAvalonConfig(t *testing.T) {
	cfg := ClassicAvalonConfig()
	if cfg.MinPlayers != 5 || cfg.MaxPlayers != 10 {
		t.Errorf("expected 5–10 players, got %d–%d", cfg.MinPlayers, cfg.MaxPlayers)
	}
	if len(cfg.Phases) == 0 {
		t.Error("expected phases")
	}
	found := false
	for _, p := range cfg.Phases {
		if p.Name == PhaseTeamSelection {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected team_selection phase")
	}
}

func TestDefaultTeamSizesForPlayerCount(t *testing.T) {
	for n, wantLen := range map[int]int{5: 5, 6: 5, 7: 5, 10: 5} {
		got := DefaultTeamSizesForPlayerCount(n)
		if len(got) != wantLen {
			t.Errorf("player count %d: got %d sizes want %d", n, len(got), wantLen)
		}
	}
}

func TestApplyMove_InvalidMoveType(t *testing.T) {
	st := &fakeGameStore{snapshot: nil}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "player-1", "invalid_type", nil)
	if result.Error == nil {
		t.Error("expected error for invalid move type")
	}
}

func TestApplyMove_BootstrapStartGame(t *testing.T) {
	players := []string{"p1", "p2", "p3", "p4", "p5"}
	st := &fakeGameStore{snapshot: nil, players: players}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "action", map[string]interface{}{"action": "start_game"})
	if result.Error != nil {
		t.Fatalf("expected success: %v", result.Error)
	}
	if result.State == nil {
		t.Fatal("expected new state")
	}
	if result.State.Phase != PhaseTeamSelection {
		t.Errorf("expected phase team_selection, got %s", result.State.Phase)
	}
	if result.State.Status != "in_progress" {
		t.Errorf("expected status in_progress, got %s", result.State.Status)
	}
	if len(result.State.PlayerIDs) != 5 {
		t.Errorf("expected 5 players, got %d", len(result.State.PlayerIDs))
	}
	if len(result.Events) != 1 || result.Events[0].Event != "game_started" {
		t.Errorf("expected game_started event, got %v", result.Events)
	}
}

func TestApplyMove_GameFinishedRejectsMove(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseFinished, Status: "finished",
		PlayerIDs: []string{"p1", "p2"}, RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "action", map[string]interface{}{"action": "propose_team", "team_ids": []string{"p1", "p2"}})
	if result.Error == nil {
		t.Error("expected error when game already finished")
	}
	if result.Error != nil && result.Error.Error() != "game already finished" {
		t.Errorf("expected 'game already finished', got %v", result.Error)
	}
}

func TestApplyMove_VoteNotAllowedInTeamSelection(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamSelection, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "vote", map[string]interface{}{"approved": true})
	if result.Error == nil {
		t.Error("expected error for vote in team_selection")
	}
}

func TestApplyMove_ProposeTeam_NonLeaderRejected(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamSelection, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	// p2 is not leader (leader is p1)
	result := engine.ApplyMove(ctx, "game-1", "p2", "action", map[string]interface{}{"action": "propose_team", "team_ids": []string{"p1", "p2"}})
	if result.Error == nil {
		t.Error("expected error when non-leader proposes team")
	}
}

func TestApplyMove_ProposeTeam_WrongSizeRejected(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamSelection, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig()) // round 1 team size 2
	ctx := context.Background()
	// round 1 requires 2 members; send 3
	result := engine.ApplyMove(ctx, "game-1", "p1", "action", map[string]interface{}{"action": "propose_team", "team_ids": []string{"p1", "p2", "p3"}})
	if result.Error == nil {
		t.Error("expected error for wrong team size")
	}
}

func TestApplyMove_ProposeTeam_Success(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamSelection, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "action", map[string]interface{}{"action": "propose_team", "team_ids": []string{"p1", "p2"}})
	if result.Error != nil {
		t.Fatalf("expected success: %v", result.Error)
	}
	if result.State.Phase != PhaseTeamVote {
		t.Errorf("expected phase team_vote, got %s", result.State.Phase)
	}
	if len(result.State.ProposedTeam) != 2 {
		t.Errorf("expected proposed_team length 2, got %d", len(result.State.ProposedTeam))
	}
	if len(result.Events) != 1 || result.Events[0].Event != "team_proposed" {
		t.Errorf("expected team_proposed event, got %v", result.Events)
	}
}

func TestApplyMove_TeamVote_InvalidPayload(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamVote, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, ProposedTeam: []string{"p1", "p2"},
		RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "vote", map[string]interface{}{})
	if result.Error == nil {
		t.Error("expected error for missing approved")
	}
}

func TestApplyMove_TeamVote_PlayerNotInGame(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamVote, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, ProposedTeam: []string{"p1", "p2"},
		RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "unknown-player", "vote", map[string]interface{}{"approved": true})
	if result.Error == nil {
		t.Error("expected error for player not in game")
	}
}

func TestApplyMove_TeamVote_Recorded(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamVote, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, ProposedTeam: []string{"p1", "p2"},
		RoundIndex: 1, LeaderIndex: 0,
	}
	st := &fakeGameStore{snapshot: state.ToMap(), players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "vote", map[string]interface{}{"approved": true})
	if result.Error != nil {
		t.Fatalf("expected success: %v", result.Error)
	}
	if result.State.TeamVotes["p1"] != "approve" {
		t.Errorf("expected p1 vote approve, got %s", result.State.TeamVotes["p1"])
	}
	if len(result.Events) != 1 || result.Events[0].Event != "vote_recorded" {
		t.Errorf("expected vote_recorded event, got %v", result.Events)
	}
}

func TestApplyMove_TeamVote_AlreadyVoted(t *testing.T) {
	state := &GameState{
		GameID: "g1", Phase: PhaseTeamVote, Status: "in_progress",
		PlayerIDs: []string{"p1", "p2", "p3", "p4", "p5"}, ProposedTeam: []string{"p1", "p2"},
		TeamVotes: map[string]string{"p1": "approve"},
		RoundIndex: 1, LeaderIndex: 0,
	}
	// Snapshot must use map[string]interface{} for nested maps so StateFromMap can parse (e.g. from JSON).
	snap := state.ToMap()
	snap["team_votes"] = map[string]interface{}{"p1": "approve"}
	st := &fakeGameStore{snapshot: snap, players: state.PlayerIDs}
	ev := &fakeEventStore{}
	engine := NewEngine(st, ev, ClassicAvalonConfig())
	ctx := context.Background()
	result := engine.ApplyMove(ctx, "game-1", "p1", "vote", map[string]interface{}{"approved": false})
	if result.Error == nil {
		t.Error("expected error for already voted")
	}
}

// Minimal fakes for engine tests without DB.
type fakeGameStore struct {
	snapshot map[string]interface{}
	players  []string
}

func (f *fakeGameStore) GetLatestSnapshot(ctx context.Context, gameID string) (map[string]interface{}, error) {
	return f.snapshot, nil
}
func (f *fakeGameStore) CreateOrUpdateSnapshot(ctx context.Context, gameID string, stateJSON map[string]interface{}) (int32, error) {
	f.snapshot = stateJSON
	return 1, nil
}
func (f *fakeGameStore) UpdateGameStatus(ctx context.Context, gameID string, status string, endedAt *time.Time) error {
	return nil
}
func (f *fakeGameStore) GetGamePlayerIDsInOrder(ctx context.Context, gameID string) ([]string, error) {
	return f.players, nil
}

type fakeEventStore struct{}

func (f *fakeEventStore) CreateGameEvent(ctx context.Context, req store.CreateGameEventRequest) (*store.GameEvent, error) {
	pl := req.Payload
	if pl == nil {
		pl = make(map[string]interface{})
	}
	return &store.GameEvent{ID: "fake-id", GameID: req.GameID, Type: req.Type, Payload: pl}, nil
}
