package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

const gatewaySettingsKey = "gateway"

// SettingsService serves the runtime switches (request logging, content
// filter) to the hot path from a short-lived in-process cache, so flipping a
// switch in the console takes effect on every instance within settingsTTL.
type SettingsService struct {
	repo domain.SettingsRepository
	ttl  time.Duration

	mu     sync.Mutex
	cached domain.GatewaySettings
	exp    time.Time
}

func NewSettingsService(repo domain.SettingsRepository, ttl time.Duration) *SettingsService {
	return &SettingsService{repo: repo, ttl: ttl}
}

// Get never fails the caller: on a database error it keeps serving the last
// known (or default) settings.
func (s *SettingsService) Get(ctx context.Context) domain.GatewaySettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.exp) {
		return s.cached
	}
	st := domain.DefaultGatewaySettings()
	if _, err := s.repo.Load(ctx, gatewaySettingsKey, &st); err != nil {
		if s.exp.IsZero() {
			s.cached = domain.DefaultGatewaySettings()
		}
		s.exp = time.Now().Add(s.ttl)
		return s.cached
	}
	s.cached, s.exp = st, time.Now().Add(s.ttl)
	return st
}

func (s *SettingsService) Update(ctx context.Context, st domain.GatewaySettings, operator string) (domain.GatewaySettings, error) {
	rl := &st.RequestLog
	if rl.MaxBodyKB < 1 || rl.MaxBodyKB > 1024 {
		return st, fmt.Errorf("%w: max_body_kb must be between 1 and 1024", domain.ErrInvalidArgument)
	}
	if rl.RetentionDays < 1 || rl.RetentionDays > 365 {
		return st, fmt.Errorf("%w: retention_days must be between 1 and 365", domain.ErrInvalidArgument)
	}
	st.ContentFilter.BlockMessage = strings.TrimSpace(st.ContentFilter.BlockMessage)
	if st.ContentFilter.BlockMessage == "" {
		st.ContentFilter.BlockMessage = domain.DefaultGatewaySettings().ContentFilter.BlockMessage
	}
	if err := s.repo.Save(ctx, gatewaySettingsKey, st, operator); err != nil {
		return st, err
	}
	s.mu.Lock()
	s.cached, s.exp = st, time.Now().Add(s.ttl)
	s.mu.Unlock()
	return st, nil
}
