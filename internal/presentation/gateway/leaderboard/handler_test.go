package leaderboard_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	leaderboardv1 "github.com/averak/vfx/gen/go/vfx/v1/leaderboard"
	"github.com/averak/vfx/internal/testutils/testconnect"
)

func login(t *testing.T, srv *testconnect.Server, device string) string {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{DeviceId: &device},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp.Msg.GetAccessToken()
}

func submit(t *testing.T, srv *testconnect.Server, token, board string, score int64) *leaderboardv1.SubmitScoreResponse {
	t.Helper()
	resp, err := srv.Leaderboard.SubmitScore(t.Context(),
		testconnect.Authorize(connect.NewRequest(&leaderboardv1.SubmitScoreRequest{LeaderboardId: board, Score: score}), token))
	if err != nil {
		t.Fatalf("SubmitScore(%s, %d): %v", board, score, err)
	}
	return resp.Msg
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want %v", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v", got, want)
	}
}

func TestSubmitScore_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Leaderboard.SubmitScore(t.Context(), connect.NewRequest(&leaderboardv1.SubmitScoreRequest{
		LeaderboardId: testconnect.LeaderboardDesc,
		Score:         1,
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestSubmitScore_UnknownLeaderboard(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "unknown-lb")
	_, err := srv.Leaderboard.SubmitScore(t.Context(),
		testconnect.Authorize(connect.NewRequest(&leaderboardv1.SubmitScoreRequest{LeaderboardId: "does-not-exist", Score: 1}), token))
	requireCode(t, err, connect.CodeNotFound)
}

// Descending keeps the max: a lower later score does not replace a higher one, and improved reflects that.
func TestSubmitScore_DescendingKeepsBest(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "desc-best")

	first := submit(t, srv, token, testconnect.LeaderboardDesc, 100)
	if !first.GetImproved() || first.GetEntry().GetScore() != 100 {
		t.Fatalf("first submit: improved=%v score=%d", first.GetImproved(), first.GetEntry().GetScore())
	}
	lower := submit(t, srv, token, testconnect.LeaderboardDesc, 50)
	if lower.GetImproved() || lower.GetEntry().GetScore() != 100 {
		t.Errorf("lower submit changed best: improved=%v score=%d", lower.GetImproved(), lower.GetEntry().GetScore())
	}
	higher := submit(t, srv, token, testconnect.LeaderboardDesc, 150)
	if !higher.GetImproved() || higher.GetEntry().GetScore() != 150 {
		t.Errorf("higher submit not recorded: improved=%v score=%d", higher.GetImproved(), higher.GetEntry().GetScore())
	}
}

// Ascending keeps the min (race times): a higher later time does not replace a lower one.
func TestSubmitScore_AscendingKeepsBest(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "asc-best")

	submit(t, srv, token, testconnect.LeaderboardAsc, 60)
	worse := submit(t, srv, token, testconnect.LeaderboardAsc, 90)
	if worse.GetImproved() || worse.GetEntry().GetScore() != 60 {
		t.Errorf("slower time replaced best: improved=%v score=%d", worse.GetImproved(), worse.GetEntry().GetScore())
	}
	better := submit(t, srv, token, testconnect.LeaderboardAsc, 45)
	if !better.GetImproved() || better.GetEntry().GetScore() != 45 {
		t.Errorf("faster time not recorded: improved=%v score=%d", better.GetImproved(), better.GetEntry().GetScore())
	}
}

func TestListRanks_OrdersByScore(t *testing.T) {
	srv := testconnect.New(t)
	submit(t, srv, login(t, srv, "p-low"), testconnect.LeaderboardDesc, 10)
	submit(t, srv, login(t, srv, "p-high"), testconnect.LeaderboardDesc, 30)
	submit(t, srv, login(t, srv, "p-mid"), testconnect.LeaderboardDesc, 20)

	resp, err := srv.Leaderboard.ListRanks(t.Context(),
		testconnect.Authorize(connect.NewRequest(&leaderboardv1.ListRanksRequest{LeaderboardId: testconnect.LeaderboardDesc}), login(t, srv, "viewer")))
	if err != nil {
		t.Fatalf("ListRanks: %v", err)
	}
	entries := resp.Msg.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	wantScores := []int64{30, 20, 10}
	for i, e := range entries {
		if e.GetScore() != wantScores[i] || e.GetRank() != int64(i+1) {
			t.Errorf("entry %d = (rank %d, score %d), want (rank %d, score %d)", i, e.GetRank(), e.GetScore(), i+1, wantScores[i])
		}
	}
}

func TestGetPlayerRank_NotFoundWithoutEntry(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "no-entry")
	_, err := srv.Leaderboard.GetPlayerRank(t.Context(),
		testconnect.Authorize(connect.NewRequest(&leaderboardv1.GetPlayerRankRequest{LeaderboardId: testconnect.LeaderboardDesc}), token))
	requireCode(t, err, connect.CodeNotFound)
}

// The caller sits in the middle of five entries; radius 1 returns the three around them with correct ranks.
func TestListRanksAroundPlayer(t *testing.T) {
	srv := testconnect.New(t)
	for i, dev := range []string{"a", "b", "c", "d", "e"} {
		submit(t, srv, login(t, srv, "around-"+dev), testconnect.LeaderboardDesc, int64(100-i*10)) // 100,90,80,70,60
	}
	// The "c" player scored 80 -> rank 3.
	cToken := login(t, srv, "around-c")

	resp, err := srv.Leaderboard.ListRanksAroundPlayer(t.Context(),
		testconnect.Authorize(connect.NewRequest(&leaderboardv1.ListRanksAroundPlayerRequest{LeaderboardId: testconnect.LeaderboardDesc, Radius: 1}), cToken))
	if err != nil {
		t.Fatalf("ListRanksAroundPlayer: %v", err)
	}
	entries := resp.Msg.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if entries[0].GetRank() != 2 || entries[1].GetRank() != 3 || entries[2].GetRank() != 4 {
		t.Errorf("ranks = %d,%d,%d, want 2,3,4", entries[0].GetRank(), entries[1].GetRank(), entries[2].GetRank())
	}
	if entries[1].GetScore() != 80 {
		t.Errorf("center score = %d, want 80", entries[1].GetScore())
	}
}
