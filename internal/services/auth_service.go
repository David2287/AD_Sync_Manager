package services

import (
	"context"
	"fmt"
	"time"

	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

type authService struct {
	ad      interfaces.ADClient
	session interfaces.SessionService
	log     interfaces.Logger
}

// NewAuthService wires the authentication use-case.
func NewAuthService(
	ad interfaces.ADClient,
	session interfaces.SessionService,
	log interfaces.Logger,
) interfaces.AuthService {
	return &authService{ad: ad, session: session, log: log.With("svc", "auth")}
}

func (s *authService) Login(ctx context.Context, req interfaces.LoginRequest) (*interfaces.LoginResponse, error) {
	groups, err := s.ad.Authenticate(ctx, req.Username, req.Password)
	if err != nil {
		s.log.Warn("login failed", "user", req.Username, "error", err)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Fetch the display name from AD so we can embed it in the JWT.
	emp, err := s.ad.GetEmployee(ctx, req.Username)
	if err != nil {
		s.log.Warn("could not fetch employee after login", "user", req.Username, "error", err)
	}

	displayName := req.Username
	if emp != nil {
		displayName = emp.DisplayName
	}

	sess := &entity.Session{
		UserID:    req.Username,
		Username:  displayName,
		Groups:    groups,
		IssuedAt:  time.Now(),
	}

	token, err := s.session.Issue(ctx, sess)
	if err != nil {
		return nil, fmt.Errorf("auth: issue token: %w", err)
	}

	s.log.Info("user logged in", "user", req.Username)
	return &interfaces.LoginResponse{Token: token, Session: sess}, nil
}

func (s *authService) Logout(ctx context.Context, token string) error {
	if err := s.session.Revoke(ctx, token); err != nil {
		return fmt.Errorf("auth: revoke token: %w", err)
	}
	s.log.Info("token revoked")
	return nil
}
