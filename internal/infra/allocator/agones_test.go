package allocator

import (
	"testing"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	"agones.dev/agones/pkg/client/clientset/versioned/fake"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// allocationReactor stands in for Agones: it fills the allocation's
// status the way the real controller would, so the allocator can be
// exercised without a cluster.
func allocationReactor(status allocationv1.GameServerAllocationStatus) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		gsa := action.(k8stesting.CreateAction).GetObject().(*allocationv1.GameServerAllocation)
		gsa.Status = status
		return true, gsa, nil
	}
}

func TestAgones_AllocateReturnsEndpoint(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "gameserverallocations", allocationReactor(
		allocationv1.GameServerAllocationStatus{
			State:          allocationv1.GameServerAllocationAllocated,
			GameServerName: "rps-xyz",
			Address:        "10.0.0.5",
			Ports:          []agonesv1.GameServerStatusPort{{Name: "default", Port: 7654}},
		}))

	a := newAgonesWithClient(client, "default")
	alloc, err := a.Allocate(t.Context(), "rps", 2)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if alloc.Endpoint != "10.0.0.5:7654" {
		t.Errorf("endpoint = %q, want 10.0.0.5:7654", alloc.Endpoint)
	}
	if alloc.MatchID == uuid.Nil {
		t.Error("empty match id")
	}
}

func TestAgones_AllocateSelectsGameMode(t *testing.T) {
	client := fake.NewSimpleClientset()
	var gotLabel string
	client.PrependReactor("create", "gameserverallocations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		gsa := action.(k8stesting.CreateAction).GetObject().(*allocationv1.GameServerAllocation)
		gotLabel = gsa.Spec.Selectors[0].MatchLabels[GameModeLabel]
		gsa.Status = allocationv1.GameServerAllocationStatus{
			State:   allocationv1.GameServerAllocationAllocated,
			Address: "10.0.0.6",
			Ports:   []agonesv1.GameServerStatusPort{{Port: 7000}},
		}
		return true, gsa, nil
	})

	a := newAgonesWithClient(client, "default")
	if _, err := a.Allocate(t.Context(), "tetris", 2); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if gotLabel != "tetris" {
		t.Errorf("selector %s = %q, want tetris", GameModeLabel, gotLabel)
	}
}

func TestAgones_AllocateErrorsWhenNoServerAvailable(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "gameserverallocations", allocationReactor(
		allocationv1.GameServerAllocationStatus{State: allocationv1.GameServerAllocationUnAllocated}))

	a := newAgonesWithClient(client, "default")
	if _, err := a.Allocate(t.Context(), "rps", 2); err == nil {
		t.Error("Allocate succeeded with no available game server, want an error")
	}
}

func TestAgones_AllocateErrorsWhenNoPorts(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "gameserverallocations", allocationReactor(
		allocationv1.GameServerAllocationStatus{
			State:   allocationv1.GameServerAllocationAllocated,
			Address: "10.0.0.7",
		}))

	a := newAgonesWithClient(client, "default")
	if _, err := a.Allocate(t.Context(), "rps", 2); err == nil {
		t.Error("Allocate succeeded with no ports, want an error")
	}
}
