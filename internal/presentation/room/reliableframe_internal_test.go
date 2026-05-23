package room

import (
	"testing"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
)

// Frames that must not be dropped (system events, full snapshots, errors)
// go over a reliable stream; high-frequency deltas stay on datagrams.
func TestReliableFrame(t *testing.T) {
	tests := []struct {
		name  string
		frame *realtimev1.Frame
		want  bool
	}{
		{"event is reliable", &realtimev1.Frame{Body: &realtimev1.Frame_Event{Event: &realtimev1.SystemEvent{Type: "game_ended"}}}, true},
		{"snapshot is reliable", &realtimev1.Frame{Body: &realtimev1.Frame_Snapshot{Snapshot: &realtimev1.StateSnapshot{}}}, true},
		{"error is reliable", &realtimev1.Frame{Body: &realtimev1.Frame_Error{Error: &realtimev1.ErrorMessage{}}}, true},
		{"delta is unreliable", &realtimev1.Frame{Body: &realtimev1.Frame_Delta{Delta: &realtimev1.StateDelta{}}}, false},
		{"input is unreliable", &realtimev1.Frame{Body: &realtimev1.Frame_Input{Input: &realtimev1.PlayerInput{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reliableFrame(tt.frame); got != tt.want {
				t.Errorf("reliableFrame(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
