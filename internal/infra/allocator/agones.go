package allocator

import (
	"context"
	"fmt"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	versioned "agones.dev/agones/pkg/client/clientset/versioned"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/averak/vfx/internal/domain/match"
)

// GameModeLabel is the label vfx stamps on a Fleet's GameServer template so the matchmaker can select a room of the right game mode.
// The Helm chart sets it; the allocator filters on it.
const GameModeLabel = "vfx.dev/game-mode"

// Agones reserves a Ready GameServer of the requested game mode via a GameServerAllocation against the in-cluster API.
type Agones struct {
	client    versioned.Interface
	namespace string
}

var _ match.Allocator = (*Agones)(nil)

// NewAgones must run inside a pod whose service account may create allocation.agones.dev/gameserverallocations (the Helm chart grants this to the gateway).
func NewAgones(namespace string) (*Agones, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("allocator: in-cluster config: %w", err)
	}
	client, err := versioned.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("allocator: agones clientset: %w", err)
	}
	return newAgonesWithClient(client, namespace), nil
}

// newAgonesWithClient is the seam for tests, which pass a fake clientset.
func newAgonesWithClient(client versioned.Interface, namespace string) *Agones {
	return &Agones{client: client, namespace: namespace}
}

func (a *Agones) Allocate(ctx context.Context, gameMode string, _ int) (*match.RoomAllocation, error) {
	ready := agonesv1.GameServerStateReady
	gsa := &allocationv1.GameServerAllocation{
		Spec: allocationv1.GameServerAllocationSpec{
			Selectors: []allocationv1.GameServerSelector{{
				LabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{GameModeLabel: gameMode},
				},
				GameServerState: &ready,
			}},
		},
	}

	result, err := a.client.AllocationV1().GameServerAllocations(a.namespace).Create(ctx, gsa, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("allocator: create allocation: %w", err)
	}
	if result.Status.State != allocationv1.GameServerAllocationAllocated {
		// No Ready GameServer was available; the matchmaker surfaces this to the waiting tickets so players can retry.
		return nil, fmt.Errorf("allocator: no game server available for %q (state=%s)", gameMode, result.Status.State)
	}
	if len(result.Status.Ports) == 0 {
		return nil, fmt.Errorf("allocator: allocated game server %q exposes no ports", result.Status.GameServerName)
	}

	return &match.RoomAllocation{
		MatchID:  uuid.New(),
		Endpoint: fmt.Sprintf("%s:%d", result.Status.Address, result.Status.Ports[0].Port),
	}, nil
}
