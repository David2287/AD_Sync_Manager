package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ad-sync-manager/internal/config"
	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

type sessionService struct {
	cfg     config.JWTConfig
	revoked sync.Map // token string → struct{} (in-memory deny-list)
}

// NewSessionService returns an interfaces.SessionService backed by JWT.
func NewSessionService(cfg config.JWTConfig) interfaces.SessionService {
	return &sessionService{cfg: cfg}
}

func (s *sessionService) Issue(_ context.Context, sess *entity.Session) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": s.cfg.Issuer,
		"sub": sess.UserID,
		"uid": sess.UserID,
		"dsp": sess.Username,
		"grp": sess.Groups,
		"iat": now.Unix(),
		"exp": now.Add(s.cfg.Expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return "", fmt.Errorf("session: sign token: %w", err)
	}
	return signed, nil
}

func (s *sessionService) Validate(_ context.Context, tokenStr string) (*entity.Session, error) {
	if _, revoked := s.revoked.Load(tokenStr); revoked {
		return nil, fmt.Errorf("session: token has been revoked")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("session: unexpected signing method %v", t.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	}, jwt.WithIssuer(s.cfg.Issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("session: invalid token: %w", err)
	}

	c, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("session: malformed claims")
	}

	userID, _ := c["uid"].(string)
	username, _ := c["dsp"].(string)

	var groups []string
	if raw, ok := c["grp"].([]any); ok {
		for _, g := range raw {
			if gs, ok := g.(string); ok {
				groups = append(groups, gs)
			}
		}
	}

	exp, _ := c.GetExpirationTime()
	iat, _ := c.GetIssuedAt()

	return &entity.Session{
		UserID:    userID,
		Username:  username,
		Groups:    groups,
		IssuedAt:  iat.Time,
		ExpiresAt: exp.Time,
	}, nil
}

func (s *sessionService) Revoke(_ context.Context, tokenStr string) error {
	s.revoked.Store(tokenStr, struct{}{})
	return nil
}
