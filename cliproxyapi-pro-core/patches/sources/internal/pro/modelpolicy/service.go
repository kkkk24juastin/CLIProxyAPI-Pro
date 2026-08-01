package modelpolicy

import (
	"context"
	"fmt"
	"sync"

	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy/config"
	modelengine "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy/policy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

type Status struct {
	Enabled        bool   `json:"enabled"`
	CacheTTL       string `json:"cacheTTL"`
	ResolveTimeout string `json:"resolveTimeout"`
	Providers      int    `json:"providers"`
	LastError      string `json:"lastError,omitempty"`
}

// Service owns OAuth account-plan model filtering and its persisted policy.
type Service struct {
	mu         sync.RWMutex
	store      settings.Store
	config     modelconfig.Config
	engine     *modelengine.Engine
	configErr  string
	onChange   func(context.Context)
	unregister func()
	closed     bool
}

func New(ctx context.Context, store settings.Store) (*Service, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return nil, fmt.Errorf("model policy settings store is required")
	}
	cfg, loadErr := loadConfig(ctx, store)
	service := &Service{store: store, config: cfg, engine: modelengine.New()}
	if loadErr != nil {
		service.configErr = loadErr.Error()
	}
	service.engine.ApplyConfig(cfg)
	service.unregister = store.Subscribe(settings.NamespaceOAuthModelPolicy, service.applyImportedSetting)
	return service, nil
}

func loadConfig(ctx context.Context, store settings.Store) (modelconfig.Config, error) {
	cfg, _ := modelconfig.Parse(nil)
	item, found, err := store.Get(ctx, settings.NamespaceOAuthModelPolicy)
	if err != nil || !found {
		return cfg, err
	}
	if item.SchemaVersion != settings.SchemaVersionOne {
		return cfg, fmt.Errorf("unsupported OAuth model policy schema version %d", item.SchemaVersion)
	}
	return modelconfig.Parse(item.Settings)
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	unregister := s.unregister
	s.unregister = nil
	s.onChange = nil
	s.mu.Unlock()
	if unregister != nil {
		unregister()
	}
}

func (s *Service) SetChangeHandler(handler func(context.Context)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.onChange = handler
	s.mu.Unlock()
}

func (s *Service) Config() modelconfig.Config {
	if s == nil {
		cfg, _ := modelconfig.Parse(nil)
		return cfg
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

func (s *Service) UpdateConfig(ctx context.Context, cfg modelconfig.Config) error {
	if s == nil {
		return fmt.Errorf("model policy service is unavailable")
	}
	raw, err := modelconfig.Marshal(cfg)
	if err != nil {
		return err
	}
	normalized, err := modelconfig.Parse(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("model policy service is closed")
	}
	if err := s.store.Put(ctx, settings.Item{
		Namespace: settings.NamespaceOAuthModelPolicy, SchemaVersion: settings.SchemaVersionOne, Settings: raw,
	}); err != nil {
		s.mu.Unlock()
		return err
	}
	s.config = normalized
	s.configErr = ""
	s.engine.ApplyConfig(normalized)
	handler := s.onChange
	s.mu.Unlock()
	if handler != nil {
		handler(ctx)
	}
	return nil
}

func (s *Service) applyImportedSetting(ctx context.Context, item settings.Item) error {
	if item.SchemaVersion != settings.SchemaVersionOne {
		return fmt.Errorf("unsupported OAuth model policy schema version %d", item.SchemaVersion)
	}
	cfg, err := modelconfig.Parse(item.Settings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("model policy service is closed")
	}
	s.config = cfg
	s.configErr = ""
	s.engine.ApplyConfig(cfg)
	handler := s.onChange
	s.mu.Unlock()
	if handler != nil {
		handler(ctx)
	}
	return nil
}

func (s *Service) Status() Status {
	if s == nil {
		return Status{LastError: "model policy service is unavailable"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Enabled: s.config.Enabled, CacheTTL: s.config.CacheTTL.String(),
		ResolveTimeout: s.config.ResolveTimeout.String(), Providers: len(s.config.Providers), LastError: s.configErr,
	}
}

func (s *Service) Filter(ctx context.Context, input modelengine.Input) modelengine.Result {
	if s == nil {
		return modelengine.Result{}
	}
	s.mu.RLock()
	engine := s.engine
	closed := s.closed
	s.mu.RUnlock()
	if closed || engine == nil {
		return modelengine.Result{}
	}
	return engine.Filter(ctx, input)
}
