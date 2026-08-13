package oauthpolicy

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/config"
	modelengine "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/policy"
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
	revision   uint64
	authEpochs map[string]uint64
	configErr  string
	effective  map[string]modelengine.EffectivePolicy
	decisions  map[string]modelengine.Result
	onChange   func(context.Context)
	unregister func()
	changeCtx  context.Context
	changeStop context.CancelFunc
	changeRun  bool
	changeNext bool
	closed     bool
}

func New(ctx context.Context, store settings.Store) (*Service, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return nil, fmt.Errorf("account policy settings store is required")
	}
	cfg, loadErr := loadConfig(ctx, store)
	changeCtx, changeStop := context.WithCancel(context.Background())
	engine := modelengine.New()
	engine.ApplyConfig(cfg)
	service := &Service{
		store: store, config: cfg, engine: engine,
		revision: 1, authEpochs: make(map[string]uint64),
		effective: make(map[string]modelengine.EffectivePolicy), decisions: make(map[string]modelengine.Result),
		changeCtx: changeCtx, changeStop: changeStop,
	}
	if loadErr != nil {
		service.configErr = loadErr.Error()
	}
	service.unregister = store.Subscribe(settings.NamespaceOAuthPolicy, service.applyImportedSetting)
	return service, nil
}

func loadConfig(ctx context.Context, store settings.Store) (modelconfig.Config, error) {
	cfg, _ := modelconfig.Parse(nil)
	item, found, err := store.Get(ctx, settings.NamespaceOAuthPolicy)
	if err != nil || !found {
		legacy, legacyFound, legacyErr := store.Get(ctx, settings.LegacyNamespaceOAuthModelPolicy)
		if legacyErr != nil || !legacyFound {
			return cfg, firstError(err, legacyErr)
		}
		legacy.Namespace = settings.NamespaceOAuthPolicy
		if errPut := store.Put(ctx, legacy); errPut != nil {
			return cfg, errPut
		}
		verified, verifiedFound, errVerify := store.Get(ctx, settings.NamespaceOAuthPolicy)
		if errVerify != nil || !verifiedFound {
			return cfg, firstError(errVerify, fmt.Errorf("verify migrated OAuth policy setting"))
		}
		if errDelete := store.Delete(ctx, settings.LegacyNamespaceOAuthModelPolicy); errDelete != nil {
			return cfg, errDelete
		}
		item = verified
	} else {
		if errDelete := store.Delete(ctx, settings.LegacyNamespaceOAuthModelPolicy); errDelete != nil {
			return cfg, errDelete
		}
	}
	if item.SchemaVersion != settings.SchemaVersionOne {
		return cfg, fmt.Errorf("unsupported OAuth account policy schema version %d", item.SchemaVersion)
	}
	return modelconfig.Parse(item.Settings)
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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
	changeStop := s.changeStop
	s.changeStop = nil
	s.mu.Unlock()
	if changeStop != nil {
		changeStop()
	}
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
		return fmt.Errorf("account policy service is unavailable")
	}
	raw, err := modelconfig.Marshal(cfg)
	if err != nil {
		return err
	}
	normalized, err := modelconfig.Parse(raw)
	if err != nil {
		return err
	}
	operation := func(ctx context.Context, store settings.Store) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return fmt.Errorf("account policy service is closed")
		}
		if err := store.Put(ctx, settings.Item{
			Namespace: settings.NamespaceOAuthPolicy, SchemaVersion: settings.SchemaVersionOne, Settings: raw,
		}); err != nil {
			return err
		}
		engine := modelengine.New()
		engine.ApplyConfig(normalized)
		s.config = normalized
		s.engine = engine
		s.revision++
		s.configErr = ""
		s.effective = make(map[string]modelengine.EffectivePolicy)
		s.decisions = make(map[string]modelengine.Result)
		return nil
	}
	if coordinator, ok := s.store.(settings.WriteCoordinator); ok {
		if err := coordinator.ExecuteWrite(ctx, operation); err != nil {
			return err
		}
	} else if err := operation(ctx, s.store); err != nil {
		return err
	}
	s.queueChange()
	return nil
}

func (s *Service) applyImportedSetting(_ context.Context, item settings.Item) error {
	if item.SchemaVersion != settings.SchemaVersionOne {
		return fmt.Errorf("unsupported OAuth account policy schema version %d", item.SchemaVersion)
	}
	cfg, err := modelconfig.Parse(item.Settings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("account policy service is closed")
	}
	engine := modelengine.New()
	engine.ApplyConfig(cfg)
	s.config = cfg
	s.engine = engine
	s.revision++
	s.configErr = ""
	s.effective = make(map[string]modelengine.EffectivePolicy)
	s.decisions = make(map[string]modelengine.Result)
	s.mu.Unlock()
	s.queueChange()
	return nil
}

// queueChange applies saved policy changes outside the management request. A
// refresh may resolve remote account plans, so waiting here would make a
// successful SQLite write look like a failed save when the client times out.
// Concurrent updates are coalesced into at most one additional refresh.
func (s *Service) queueChange() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed || s.onChange == nil || s.changeCtx == nil {
		s.mu.Unlock()
		return
	}
	s.changeNext = true
	if s.changeRun {
		s.mu.Unlock()
		return
	}
	s.changeRun = true
	ctx := s.changeCtx
	s.mu.Unlock()
	go s.runChanges(ctx)
}

func (s *Service) runChanges(ctx context.Context) {
	for {
		s.mu.Lock()
		if s.closed || !s.changeNext || ctx.Err() != nil {
			s.changeRun = false
			s.mu.Unlock()
			return
		}
		s.changeNext = false
		handler := s.onChange
		s.mu.Unlock()
		if handler != nil {
			handler(ctx)
		}
	}
}

func (s *Service) Status() Status {
	if s == nil {
		return Status{LastError: "account policy service is unavailable"}
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
	for {
		s.mu.RLock()
		engine := s.engine
		closed := s.closed
		revision := s.revision
		authEpoch := s.authEpochs[input.AuthID]
		s.mu.RUnlock()
		if closed || engine == nil {
			return modelengine.Result{}
		}
		result := engine.Filter(ctx, input)
		s.mu.Lock()
		if s.closed || s.authEpochs[input.AuthID] != authEpoch {
			s.mu.Unlock()
			return modelengine.Result{}
		}
		if s.revision != revision {
			s.mu.Unlock()
			if ctx != nil && ctx.Err() != nil {
				return modelengine.Result{}
			}
			continue
		}
		if result.Handled {
			s.decisions[input.AuthID] = result
			s.effective[input.AuthID] = modelengine.EffectivePolicy{
				AuthID: input.AuthID, Provider: input.AuthProvider,
				PlanKey: result.Annotations["plan_key"], PlanSource: result.Annotations["plan_source"],
				MatchedRule: result.Annotations["matched_rule"], PlanError: result.Annotations["plan_error"],
				Prefix: effectivePrefix(input, result), Priority: effectivePriority(input, result), Weight: effectiveWeight(input, result),
				ExcludedCount: len(result.ExcludedModelIDs),
			}
		} else {
			delete(s.effective, input.AuthID)
			delete(s.decisions, input.AuthID)
		}
		s.mu.Unlock()
		return result
	}
}

// ForgetAuth removes runtime-only policy state for an authentication account.
// Advancing the account epoch also prevents a provider lookup that started
// before deletion from restoring the removed decision after it completes.
func (s *Service) ForgetAuth(authID string) {
	if s == nil || strings.TrimSpace(authID) == "" {
		return
	}
	s.mu.Lock()
	if !s.closed {
		s.authEpochs[authID]++
		if s.engine != nil {
			s.engine.ForgetAuth(authID)
		}
		delete(s.effective, authID)
		delete(s.decisions, authID)
	}
	s.mu.Unlock()
}

func effectivePrefix(input modelengine.Input, result modelengine.Result) string {
	if result.Prefix != nil {
		return *result.Prefix
	}
	return strings.TrimSpace(input.AuthPrefix)
}

func effectivePriority(input modelengine.Input, result modelengine.Result) int {
	if result.Priority != nil {
		return *result.Priority
	}
	value, _ := strconv.Atoi(strings.TrimSpace(input.Attributes["priority"]))
	return value
}

func effectiveWeight(input modelengine.Input, result modelengine.Result) int64 {
	if result.Weight != nil {
		return *result.Weight
	}
	value := int64(1)
	if parsed, err := strconv.ParseInt(strings.TrimSpace(input.Attributes["weight"]), 10, 64); err == nil {
		value = parsed
	}
	return value
}

func (s *Service) EffectivePolicies() []modelengine.EffectivePolicy {
	if s == nil {
		return []modelengine.EffectivePolicy{}
	}
	s.mu.RLock()
	out := make([]modelengine.EffectivePolicy, 0, len(s.effective))
	for _, policy := range s.effective {
		out = append(out, policy)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].AuthID < out[j].AuthID
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func (s *Service) EffectivePolicy(authID string) (modelengine.Result, bool) {
	if s == nil || authID == "" {
		return modelengine.Result{}, false
	}
	s.mu.RLock()
	policy, found := s.decisions[authID]
	s.mu.RUnlock()
	return policy, found
}
