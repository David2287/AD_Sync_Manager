package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// SessionService is the port for JWT lifecycle management.
type SessionService interface {
	// Issue signs and returns a JWT for the given session.
	Issue(ctx context.Context, session *entity.Session) (token string, err error)

	// Validate parses and verifies a JWT string, returning its session payload.
	Validate(ctx context.Context, token string) (*entity.Session, error)

	// Revoke adds the token to the deny-list (in-memory map in Stage 1;
	// Redis or DB-backed in production).
	Revoke(ctx context.Context, token string) error
}
