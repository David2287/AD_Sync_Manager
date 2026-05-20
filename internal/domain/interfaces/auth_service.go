package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// LoginRequest carries credentials from the HTTP handler.
type LoginRequest struct {
	Username string
	Password string
}

// LoginResponse carries the signed JWT and basic session metadata.
type LoginResponse struct {
	Token     string
	Session   *entity.Session
}

// AuthService is the port for the authentication use-case.
type AuthService interface {
	Login(ctx context.Context, req LoginRequest) (*LoginResponse, error)
	Logout(ctx context.Context, token string) error
}
