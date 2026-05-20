package auth

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/internal/domain/player"
)

// toPlayerPb converts a domain Player to the proto representation.
func toPlayerPb(p *player.Player) *authv1.Player {
	return &authv1.Player{
		Id:        p.ID.String(),
		Nickname:  p.Nickname,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}
