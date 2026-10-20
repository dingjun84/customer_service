package admin

import (
	"context"
	"time"
)

// Metadata describes the running process for the health card. It is filled by
// the composition root so the console never re-reads configuration itself.
type Metadata struct {
	SchemaVersion int
	DBPath        string
	AppEnv        string
	LLMMode       string
	ExposeDBPath  bool
	SessionTTL    time.Duration
}

// Service is the console's application boundary: one type for the HTTP layer to
// depend on. It embeds Store so read models are promoted directly — there is no
// business rule to add between a staff query and its SQL — while Auth stays a
// separate field because logging in is not a row query.
type Service struct {
	*Store
	Auth *Auth
	meta Metadata
}

// NewService wires the store, the auth service and the process metadata.
func NewService(store *Store, auth *Auth, meta Metadata) *Service {
	return &Service{Store: store, Auth: auth, meta: meta}
}

// Metadata returns the process facts the console displays.
func (s *Service) Metadata() Metadata { return s.meta }

// Health builds the data-health card: schema version, per-table row counts and
// the live session count.
func (s *Service) Health(ctx context.Context) (Health, error) {
	dbPath := ""
	if s.meta.ExposeDBPath {
		dbPath = s.meta.DBPath
	}
	h, err := s.Store.Health(ctx, s.meta.SchemaVersion, dbPath, s.meta.AppEnv, s.meta.LLMMode)
	if err != nil {
		return Health{}, err
	}
	sessions, err := s.Auth.ActiveSessions(ctx)
	if err != nil {
		return Health{}, err
	}
	h.ActiveSessions = sessions
	return h, nil
}

// Overview builds the dashboard.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	h, err := s.Health(ctx)
	if err != nil {
		return Overview{}, err
	}
	ov, err := s.Store.Overview(ctx, h)
	if err != nil {
		return Overview{}, err
	}
	// The dashboard should not show a stale session count from before the last
	// login; Health already read it fresh, so reuse that value.
	return ov, nil
}
