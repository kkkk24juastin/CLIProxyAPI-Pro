package apikeypolicy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	probackup "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/backup"
)

type runtimeIndex struct {
	healthy         bool
	takeoverEnabled bool
	generation      uint64
	items           map[string]RequestPolicySnapshot
}

type backupPolicy struct {
	ID              string    `json:"id"`
	APIKeyHash      string    `json:"api_key_hash"`
	DisplayName     string    `json:"display_name"`
	ActiveProfileID string    `json:"active_profile_id"`
	Version         int64     `json:"version"`
	CreatedAtMS     int64     `json:"created_at_ms"`
	UpdatedAtMS     int64     `json:"updated_at_ms"`
	Profiles        []Profile `json:"profiles"`
}

type backupDocument struct {
	SchemaVersion   int            `json:"schema_version"`
	TakeoverEnabled bool           `json:"takeover_enabled"`
	Policies        []backupPolicy `json:"policies"`
	Audits          []AuditRecord  `json:"audits"`
}

func policiesToBackup(policies []Policy) []backupPolicy {
	out := make([]backupPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, backupPolicy{
			ID: policy.ID, APIKeyHash: policy.APIKeyHash, DisplayName: policy.DisplayName,
			ActiveProfileID: policy.ActiveProfileID, Version: policy.Version,
			CreatedAtMS: policy.CreatedAtMS, UpdatedAtMS: policy.UpdatedAtMS,
			Profiles: policy.Profiles,
		})
	}
	return out
}

func backupToPolicies(items []backupPolicy) []Policy {
	out := make([]Policy, 0, len(items))
	for _, item := range items {
		out = append(out, Policy{
			ID: item.ID, APIKeyHash: item.APIKeyHash, DisplayName: item.DisplayName,
			ActiveProfileID: item.ActiveProfileID, Version: item.Version,
			CreatedAtMS: item.CreatedAtMS, UpdatedAtMS: item.UpdatedAtMS,
			Profiles: item.Profiles,
		})
	}
	return out
}

func (s *Service) ExportBackup(ctx context.Context) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	policies, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	audits, err := s.store.ListAudits(ctx)
	if err != nil {
		return nil, err
	}
	takeoverEnabled, err := s.store.TakeoverEnabled(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(backupDocument{SchemaVersion: 3, TakeoverEnabled: takeoverEnabled, Policies: policiesToBackup(policies), Audits: audits})
}

func decodeBackup(payload []byte) ([]Policy, []AuditRecord, bool, error) {
	var document backupDocument
	if err := json.Unmarshal(payload, &document); err == nil && document.SchemaVersion != 0 {
		if document.SchemaVersion != 2 && document.SchemaVersion != 3 {
			return nil, nil, false, fmt.Errorf("unsupported API key policy backup schema %d", document.SchemaVersion)
		}
		return backupToPolicies(document.Policies), document.Audits, document.SchemaVersion >= 3 && document.TakeoverEnabled, nil
	}
	// Version 1 was a bare policy array. It remains importable and contains no
	// audit history by definition.
	var legacy []backupPolicy
	if err := json.Unmarshal(payload, &legacy); err != nil {
		return nil, nil, false, err
	}
	return backupToPolicies(legacy), nil, false, nil
}

func validateBackupAudits(items []AuditRecord) error {
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.ID <= 0 || strings.TrimSpace(item.PolicyID) == "" || strings.TrimSpace(item.EventType) == "" || item.CreatedAtMS <= 0 || !json.Valid(item.Details) {
			return fmt.Errorf("invalid API key policy audit backup record")
		}
		if _, exists := seen[item.ID]; exists {
			return fmt.Errorf("duplicate API key policy audit ID %d", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func profileCount(policies []Policy) int {
	count := 0
	for _, policy := range policies {
		count += len(policy.Profiles)
	}
	return count
}

// PreviewBackup performs the same validation and immutable-index construction
// as import, then derives association counts from the committed config key
// fingerprints supplied by the Management handler.
func (s *Service) PreviewBackup(ctx context.Context, payload []byte, configuredHashes []string) (probackup.PolicyBackupPreview, error) {
	policies, _, targetTakeoverEnabled, _, err := s.stageBackup(payload)
	if err != nil {
		return probackup.PolicyBackupPreview{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	current, err := s.store.List(ctx)
	if err != nil {
		return probackup.PolicyBackupPreview{}, err
	}
	currentTakeoverEnabled, err := s.store.TakeoverEnabled(ctx)
	if err != nil {
		return probackup.PolicyBackupPreview{}, err
	}
	configured := make(map[string]struct{}, len(configuredHashes))
	for _, hash := range configuredHashes {
		configured[hash] = struct{}{}
	}
	preview := probackup.PolicyBackupPreview{
		HasPolicies:            true,
		ReplacePolicies:        len(current),
		ReplaceProfiles:        profileCount(current),
		TargetPolicies:         len(policies),
		TargetProfiles:         profileCount(policies),
		CurrentTakeoverEnabled: currentTakeoverEnabled,
		TargetTakeoverEnabled:  targetTakeoverEnabled,
	}
	for _, policy := range policies {
		if _, ok := configured[policy.APIKeyHash]; ok {
			preview.AssociatedPolicies++
		} else {
			preview.OrphanedPolicies++
		}
	}
	return preview, nil
}

// ImportBackup atomically replaces the policy domain after fully validating
// staged records and building the immutable target index. It is called while
// the backup coordinator owns the global write barrier.
func (s *Service) ImportBackup(ctx context.Context, payload []byte) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	policies, audits, takeoverEnabled, next, err := s.stageBackup(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx := probackup.Transaction(ctx)
	owned := tx == nil
	if owned {
		tx, err = s.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}
	if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = null`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from api_key_policies`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from api_key_policy_audit`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update api_key_policy_settings set takeover_enabled = ? where id = 1`, takeoverEnabled); err != nil {
		return err
	}
	for _, policy := range policies {
		if _, err := tx.ExecContext(ctx, `insert into api_key_policies(id, api_key_hash, display_name, active_profile_id, version, created_at_ms, updated_at_ms) values(?, ?, ?, null, ?, ?, ?)`, policy.ID, policy.APIKeyHash, policy.DisplayName, policy.Version, policy.CreatedAtMS, policy.UpdatedAtMS); err != nil {
			return err
		}
		for _, profile := range policy.Profiles {
			input := ProfileInput{Name: profile.Name, Providers: profile.Providers, Models: profile.Models, Mappings: profile.Mappings}
			if _, err := tx.ExecContext(ctx, `insert into api_key_profiles(id, policy_id, name, created_at_ms, updated_at_ms) values(?, ?, ?, ?, ?)`, profile.ID, policy.ID, input.Name, profile.CreatedAtMS, profile.UpdatedAtMS); err != nil {
				return err
			}
			if err := replaceProfileRules(ctx, tx, profile.ID, input); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = ? where id = ?`, policy.ActiveProfileID, policy.ID); err != nil {
			return err
		}
	}
	for _, audit := range audits {
		if _, err := tx.ExecContext(ctx, `insert into api_key_policy_audit(id, policy_id, event_type, details_json, created_at_ms) values(?, ?, ?, ?, ?)`, audit.ID, audit.PolicyID, audit.EventType, string(audit.Details), audit.CreatedAtMS); err != nil {
			return err
		}
	}
	loaded, err := listPolicies(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := buildRuntimeIndex(loaded, takeoverEnabled); err != nil {
		return err
	}
	if owned {
		if err := tx.Commit(); err != nil {
			return err
		}
		s.publishNextLocked(next)
	} else {
		probackup.AfterCommit(ctx, func() {
			s.writeMu.Lock()
			defer s.writeMu.Unlock()
			s.publishNextLocked(next)
		})
	}
	return nil
}

// stageBackup is the single canonical boundary shared by preview, runtime-index
// construction and persistence. The returned policies contain exactly the
// normalized values that a successful import will publish and store.
func (s *Service) stageBackup(payload []byte) ([]Policy, []AuditRecord, bool, *runtimeIndex, error) {
	policies, audits, takeoverEnabled, err := decodeBackup(payload)
	if err != nil {
		return nil, nil, false, nil, err
	}
	policies, err = s.normalizeBackupPolicies(policies)
	if err != nil {
		return nil, nil, false, nil, err
	}
	if err = validateBackupAudits(audits); err != nil {
		return nil, nil, false, nil, err
	}
	next, err := buildRuntimeIndex(policies, takeoverEnabled)
	if err != nil {
		return nil, nil, false, nil, err
	}
	return policies, audits, takeoverEnabled, next, nil
}

func (s *Service) normalizeBackupPolicies(policies []Policy) ([]Policy, error) {
	canonical := append([]Policy(nil), policies...)
	policyIDs := make(map[string]struct{}, len(policies))
	hashes := make(map[string]struct{}, len(policies))
	profileIDs := make(map[string]struct{})
	for policyIndex := range canonical {
		policy := &canonical[policyIndex]
		if strings.TrimSpace(policy.ID) == "" || policy.Version <= 0 || len(policy.APIKeyHash) != 64 {
			return nil, fmt.Errorf("invalid API key policy backup record %q", policy.ID)
		}
		if _, err := hex.DecodeString(policy.APIKeyHash); err != nil {
			return nil, fmt.Errorf("invalid API key hash for policy %q", policy.ID)
		}
		if _, exists := policyIDs[policy.ID]; exists {
			return nil, fmt.Errorf("duplicate API key policy ID %q", policy.ID)
		}
		if _, exists := hashes[policy.APIKeyHash]; exists {
			return nil, fmt.Errorf("duplicate API key hash")
		}
		policyIDs[policy.ID] = struct{}{}
		hashes[policy.APIKeyHash] = struct{}{}
		activeCount := 0
		policy.Profiles = append([]Profile(nil), policy.Profiles...)
		for profileIndex := range policy.Profiles {
			profile := &policy.Profiles[profileIndex]
			if strings.TrimSpace(profile.ID) == "" || profile.PolicyID != policy.ID {
				return nil, fmt.Errorf("invalid profile ownership in policy %q", policy.ID)
			}
			if _, exists := profileIDs[profile.ID]; exists {
				return nil, fmt.Errorf("duplicate API key profile ID %q", profile.ID)
			}
			profileIDs[profile.ID] = struct{}{}
			input := ProfileInput{Name: profile.Name, Providers: profile.Providers, Models: profile.Models, Mappings: profile.Mappings}
			// Backups are durable policy state, so restoring them must not depend on
			// which provider credentials happen to be registered at restore time.
			// Keep structural normalization here; the live catalog still validates
			// new and edited Profiles through normalizeProfileForWrite.
			normalized, err := normalizeProfileInput(input)
			if err != nil {
				return nil, fmt.Errorf("profile %q: %w", profile.ID, err)
			}
			profile.Name = normalized.Name
			profile.Providers = normalized.Providers
			profile.Models = normalized.Models
			profile.Mappings = normalized.Mappings
			if profile.ID == policy.ActiveProfileID {
				activeCount++
			}
		}
		if len(policy.Profiles) == 0 || activeCount != 1 {
			return nil, fmt.Errorf("policy %q must have exactly one active profile", policy.ID)
		}
	}
	return canonical, nil
}

type Service struct {
	store                *Store
	writeMu              sync.Mutex
	catalogMu            sync.RWMutex
	catalogProvider      func() (ProfileCatalog, error)
	configuredMu         sync.RWMutex
	configuredHashes     atomic.Value
	configuredGeneration atomic.Uint64
	index                atomic.Pointer[runtimeIndex]
}

func (s *Service) SetConfiguredAPIKeys(keys []string) {
	if s == nil {
		return
	}
	hashes := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		identity, err := NewAuthenticatedAPIKeyIdentity(strings.TrimSpace(key))
		if err != nil {
			continue
		}
		if _, ok := seen[identity.Hash()]; ok {
			continue
		}
		seen[identity.Hash()] = struct{}{}
		hashes = append(hashes, identity.Hash())
	}
	s.configuredMu.Lock()
	s.configuredHashes.Store(hashes)
	s.configuredGeneration.Add(1)
	s.configuredMu.Unlock()
}

func (s *Service) ConfiguredGeneration() uint64 {
	if s == nil {
		return 0
	}
	return s.configuredGeneration.Load()
}

func (s *Service) ConfiguredHashes() []string {
	if s == nil {
		return nil
	}
	value := s.configuredHashes.Load()
	if value == nil {
		return nil
	}
	return append([]string(nil), value.([]string)...)
}

func (s *Service) configuredHashExists(hash string) bool {
	value := s.configuredHashes.Load()
	if value == nil {
		return false
	}
	for _, configuredHash := range value.([]string) {
		if configuredHash == hash {
			return true
		}
	}
	return false
}

// SetCatalogProvider installs the server-owned provider/model catalog used by
// all subsequent Profile writes. Existing persisted policies remain loadable
// when runtime availability changes; execution still applies live carriage
// checks for every request.
func (s *Service) SetCatalogProvider(provider func() (ProfileCatalog, error)) {
	if s == nil {
		return
	}
	s.catalogMu.Lock()
	s.catalogProvider = provider
	s.catalogMu.Unlock()
}

func (s *Service) Catalog() (ProfileCatalog, error) {
	if s == nil {
		return ProfileCatalog{}, ErrUnavailable
	}
	s.catalogMu.RLock()
	provider := s.catalogProvider
	s.catalogMu.RUnlock()
	if provider == nil {
		return ProfileCatalog{}, ErrUnavailable
	}
	catalog, err := provider()
	if err != nil {
		return ProfileCatalog{}, err
	}
	return NewProfileCatalog(catalog.Providers, catalog.Models, catalog.ModelProviders), nil
}

func (s *Service) normalizeProfileForWrite(ctx context.Context, input ProfileInput) (ProfileInput, error) {
	normalized, err := normalizeProfileInput(input)
	if err != nil {
		return ProfileInput{}, err
	}
	s.catalogMu.RLock()
	provider := s.catalogProvider
	s.catalogMu.RUnlock()
	if provider == nil {
		return normalized, nil
	}
	catalog, err := provider()
	if err != nil {
		return ProfileInput{}, fmt.Errorf("load API key policy catalog: %w", err)
	}
	if err := catalog.Validate(normalized); err != nil {
		return ProfileInput{}, err
	}
	if providerModelLinkageValidationEnabled(ctx) {
		if err := catalog.ValidateProviderModelLinkage(normalized); err != nil {
			return ProfileInput{}, err
		}
	}
	return normalized, nil
}

func NewService(store *Store) (*Service, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("api key policy store is required")
	}
	service := &Service{store: store}
	service.index.Store(&runtimeIndex{healthy: false})
	if err := service.Reload(context.Background()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Service) Healthy() bool {
	index := s.index.Load()
	return index != nil && index.healthy
}

func (s *Service) MarkUnavailable() {
	if s != nil {
		current := s.index.Load()
		takeoverEnabled := current != nil && current.takeoverEnabled
		generation := uint64(0)
		if current != nil {
			generation = current.generation
		}
		s.index.Store(&runtimeIndex{healthy: false, takeoverEnabled: takeoverEnabled, generation: generation})
	}
}

func (s *Service) Reload(ctx context.Context) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.reloadLocked(ctx)
}

func (s *Service) reloadLocked(ctx context.Context) error {
	takeoverEnabled, err := s.store.TakeoverEnabled(ctx)
	if err != nil {
		s.MarkUnavailable()
		return err
	}
	policies, err := s.store.List(ctx)
	if err != nil {
		s.MarkUnavailable()
		return err
	}
	index, err := buildRuntimeIndex(policies, takeoverEnabled)
	if err != nil {
		s.MarkUnavailable()
		return err
	}
	s.publishNextLocked(index)
	return nil
}

func nextRuntimeGeneration(current *runtimeIndex) uint64 {
	if current == nil || current.generation == ^uint64(0) {
		return 1
	}
	return current.generation + 1
}

// publishNextLocked publishes a fully built immutable index. Callers must hold
// writeMu so policy mutations and takeover confirmations share one generation.
func (s *Service) publishNextLocked(next *runtimeIndex) {
	if next == nil {
		return
	}
	next.generation = nextRuntimeGeneration(s.index.Load())
	s.index.Store(next)
}

func buildRuntimeIndex(policies []Policy, takeoverEnabled bool) (*runtimeIndex, error) {
	index := &runtimeIndex{healthy: true, takeoverEnabled: takeoverEnabled, items: make(map[string]RequestPolicySnapshot, len(policies))}
	for _, policy := range policies {
		if policy.APIKeyHash == "" || policy.ActiveProfileID == "" || policy.Version <= 0 {
			return nil, fmt.Errorf("policy %q is incomplete", policy.ID)
		}
		var active *Profile
		for profileIndex := range policy.Profiles {
			profile := &policy.Profiles[profileIndex]
			if profile.ID == policy.ActiveProfileID {
				active = profile
				break
			}
		}
		if active == nil {
			return nil, fmt.Errorf("policy %q active profile is missing", policy.ID)
		}
		input, err := normalizeProfileInput(ProfileInput{Name: active.Name, Providers: active.Providers, Models: active.Models, Mappings: active.Mappings})
		if err != nil {
			return nil, fmt.Errorf("policy %q active profile: %w", policy.ID, err)
		}
		snapshot := RequestPolicySnapshot{
			PolicyID: policy.ID, APIKeyHash: policy.APIKeyHash,
			ProfileID: active.ID, ProfileName: active.Name, Version: policy.Version,
			ModelMappings:    make(map[string]string, len(input.Mappings)),
			AllowedModels:    make(map[string]struct{}, len(input.Models)),
			AllowedProviders: make(map[string]struct{}, len(input.Providers)),
		}
		for _, mapping := range input.Mappings {
			snapshot.ModelMappings[mapping.Source] = mapping.Target
		}
		for _, model := range input.Models {
			snapshot.AllowedModels[model] = struct{}{}
		}
		for _, provider := range input.Providers {
			snapshot.AllowedProviders[provider] = struct{}{}
		}
		index.items[policy.APIKeyHash] = snapshot
	}
	return index, nil
}

func (s *Service) Decide(identity AuthenticatedAPIKeyIdentity) (RequestPolicyDecision, error) {
	if s == nil || !identity.Valid() {
		return RequestPolicyDecision{}, ErrUnavailable
	}
	index := s.index.Load()
	if index != nil && !index.takeoverEnabled {
		return PassthroughDecision(), nil
	}
	if index == nil || !index.healthy {
		return RequestPolicyDecision{}, ErrUnavailable
	}
	snapshot, found := index.items[identity.Hash()]
	if !found {
		return PassthroughDecision(), nil
	}
	cloned := snapshot.Clone()
	return RequestPolicyDecision{Mode: ModeProfile, Snapshot: &cloned}, nil
}

func (s *Service) TakeoverEnabled() bool {
	index := s.index.Load()
	return index != nil && index.takeoverEnabled
}

func (s *Service) PolicyGeneration() uint64 {
	index := s.index.Load()
	if index == nil {
		return 0
	}
	return index.generation
}

// SetTakeover changes the runtime enforcement boundary for new requests. An
// in-flight request keeps its immutable decision; disabling takeover makes all
// subsequent authenticated upstream keys use normal passthrough behavior.
func (s *Service) SetTakeover(ctx context.Context, enabled bool) error {
	return s.setTakeover(ctx, enabled, 0, 0, false)
}

// SetTakeoverIfGeneration binds takeover activation to the policy snapshot the
// operator confirmed. Emergency disable remains available without a healthy
// policy index and therefore does not require a matching generation.
func (s *Service) SetTakeoverIfGeneration(ctx context.Context, enabled bool, expectedPolicyGeneration, expectedConfiguredGeneration uint64) error {
	return s.setTakeover(ctx, enabled, expectedPolicyGeneration, expectedConfiguredGeneration, enabled)
}

func (s *Service) setTakeover(ctx context.Context, enabled bool, expectedPolicyGeneration, expectedConfiguredGeneration uint64, requireGeneration bool) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	return probackup.Default.ExecuteWrite(ctx, func(ctx context.Context) error {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		current := s.index.Load()
		if requireGeneration {
			if current == nil || !current.healthy {
				return ErrUnavailable
			}
			if current.generation != expectedPolicyGeneration {
				return ErrTakeoverStateChanged
			}
		}
		if requireGeneration {
			// Hold the configured-key snapshot stable through the database commit
			// and runtime publication. Config reloads publish through the matching
			// write lock, so activation is bound to exactly the scope confirmed by
			// the operator without taking the Management handler lock.
			s.configuredMu.RLock()
			defer s.configuredMu.RUnlock()
			if s.configuredGeneration.Load() != expectedConfiguredGeneration {
				return ErrTakeoverStateChanged
			}
		}
		tx, err := s.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `update api_key_policy_settings set takeover_enabled = ? where id = 1`, enabled); err != nil {
			return err
		}
		var next *runtimeIndex
		if enabled {
			policies, listErr := listPolicies(ctx, tx)
			if listErr != nil {
				return listErr
			}
			next, err = buildRuntimeIndex(policies, true)
			if err != nil {
				return err
			}
		} else {
			next = &runtimeIndex{takeoverEnabled: false}
			if current != nil {
				next.healthy = current.healthy
				next.items = current.items
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		s.publishNextLocked(next)
		return nil
	})
}

func (s *Service) List(ctx context.Context) ([]Policy, error) { return s.store.List(ctx) }
func (s *Service) ListProfileCatalog(ctx context.Context) (ProfileCatalogSnapshot, error) {
	if s == nil || s.store == nil {
		return ProfileCatalogSnapshot{}, ErrUnavailable
	}
	// Keep the rows and generation from one committed service state so callers
	// never cache newer names under an older generation (or the reverse).
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !s.Healthy() {
		return ProfileCatalogSnapshot{}, ErrUnavailable
	}
	items, err := s.store.ListProfileCatalog(ctx)
	if err != nil {
		return ProfileCatalogSnapshot{}, err
	}
	return ProfileCatalogSnapshot{Items: items, PolicyGeneration: s.PolicyGeneration()}, nil
}
func (s *Service) ListConfigured(ctx context.Context, configuredHashes []string) ([]Policy, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	return s.store.ListMatchingHashes(ctx, configuredHashes)
}
func (s *Service) ListOrphanedPage(ctx context.Context, configuredHashes []string, afterCreatedAtMS int64, afterID string, limit int) ([]Policy, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	return s.store.ListExcludingHashes(ctx, configuredHashes, afterCreatedAtMS, afterID, limit)
}
func (s *Service) Get(ctx context.Context, policyID string) (Policy, error) {
	return s.store.Get(ctx, policyID)
}

func (s *Service) Create(ctx context.Context, identity AuthenticatedAPIKeyIdentity, displayName string, initial ProfileInput) (Policy, error) {
	if !identity.Valid() {
		return Policy{}, errors.New("authenticated api key identity is required")
	}
	input, err := s.normalizeProfileForWrite(ctx, initial)
	if err != nil {
		return Policy{}, err
	}
	displayName = strings.TrimSpace(displayName)
	policyID, err := randomID("key_policy_")
	if err != nil {
		return Policy{}, err
	}
	profileID, err := randomID("key_profile_")
	if err != nil {
		return Policy{}, err
	}
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `insert into api_key_policies(id, api_key_hash, display_name, active_profile_id, version, created_at_ms, updated_at_ms) values(?, ?, ?, null, 1, ?, ?)`, policyID, identity.Hash(), displayName, now, now); err != nil {
			return sqliteConstraint(err)
		}
		if err := insertProfile(ctx, tx, policyID, profileID, input, now); err != nil {
			return sqliteConstraint(err)
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = ? where id = ?`, profileID, policyID); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, policyID, "policy_created", map[string]any{"activeProfileId": profileID}, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) UpdateDisplayName(ctx context.Context, policyID, displayName string, version int64) (Policy, error) {
	return s.UpdateWorkspace(ctx, policyID, version, WorkspaceUpdate{DisplayName: displayName})
}

// UpdateWorkspace changes the display name and, optionally, creates or fully
// replaces one Profile in the same optimistic-concurrency transaction. This
// prevents the Management UI from exposing a partially saved workspace when
// both fields were edited before one Save action.
func (s *Service) UpdateWorkspace(ctx context.Context, policyID string, version int64, update WorkspaceUpdate) (Policy, error) {
	update.DisplayName = strings.TrimSpace(update.DisplayName)
	update.ProfileID = strings.TrimSpace(update.ProfileID)
	if update.Profile == nil && (update.CreateProfile || update.ProfileID != "") {
		return Policy{}, errors.New("profile payload is required")
	}
	if update.Profile != nil && update.CreateProfile && update.ProfileID != "" {
		return Policy{}, errors.New("new profile must not include a profile ID")
	}
	if update.Profile != nil && !update.CreateProfile && update.ProfileID == "" {
		return Policy{}, errors.New("existing profile ID is required")
	}
	var input ProfileInput
	var err error
	if update.Profile != nil {
		input, err = s.normalizeProfileForWrite(ctx, *update.Profile)
		if err != nil {
			return Policy{}, err
		}
	}
	profileID := update.ProfileID
	if update.CreateProfile {
		profileID, err = randomID("key_profile_")
		if err != nil {
			return Policy{}, err
		}
	}
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		if update.Profile != nil {
			if update.CreateProfile {
				if err := insertProfile(ctx, tx, policyID, profileID, input, now); err != nil {
					return sqliteConstraint(err)
				}
			} else {
				result, errUpdate := tx.ExecContext(ctx, `update api_key_profiles set name = ?, updated_at_ms = ? where id = ? and policy_id = ?`, input.Name, now, profileID, policyID)
				if errUpdate != nil {
					return sqliteConstraint(errUpdate)
				}
				if count, _ := result.RowsAffected(); count != 1 {
					return ErrProfileNotFound
				}
				if err := replaceProfileRules(ctx, tx, profileID, input); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set display_name = ?, version = version + 1, updated_at_ms = ? where id = ?`, update.DisplayName, now, policyID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) CreateProfile(ctx context.Context, policyID string, version int64, input ProfileInput) (Policy, error) {
	input, err := s.normalizeProfileForWrite(ctx, input)
	if err != nil {
		return Policy{}, err
	}
	profileID, err := randomID("key_profile_")
	if err != nil {
		return Policy{}, err
	}
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		if err := insertProfile(ctx, tx, policyID, profileID, input, now); err != nil {
			return sqliteConstraint(err)
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set version = version + 1, updated_at_ms = ? where id = ?`, now, policyID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) ReplaceProfile(ctx context.Context, policyID, profileID string, version int64, input ProfileInput) (Policy, error) {
	input, err := s.normalizeProfileForWrite(ctx, input)
	if err != nil {
		return Policy{}, err
	}
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `update api_key_profiles set name = ?, updated_at_ms = ? where id = ? and policy_id = ?`, input.Name, now, profileID, policyID)
		if err != nil {
			return sqliteConstraint(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return ErrProfileNotFound
		}
		if err := replaceProfileRules(ctx, tx, profileID, input); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set version = version + 1, updated_at_ms = ? where id = ?`, now, policyID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) ActivateProfile(ctx context.Context, policyID, profileID string, version int64) (Policy, error) {
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, `select count(*) from api_key_profiles where id = ? and policy_id = ?`, profileID, policyID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return ErrProfileNotFound
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = ?, version = version + 1, updated_at_ms = ? where id = ?`, profileID, now, policyID); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, policyID, "active_profile_changed", map[string]any{"profileId": profileID}, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) DeleteProfile(ctx context.Context, policyID, profileID string, version int64) (Policy, error) {
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		var active string
		var count int
		if err := tx.QueryRowContext(ctx, `select active_profile_id, (select count(*) from api_key_profiles where policy_id = ?) from api_key_policies where id = ?`, policyID, policyID).Scan(&active, &count); err != nil {
			return err
		}
		if active == profileID {
			return ErrActiveProfileDelete
		}
		if count <= 1 {
			return ErrLastProfileDelete
		}
		result, err := tx.ExecContext(ctx, `delete from api_key_profiles where id = ? and policy_id = ?`, profileID, policyID)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return ErrProfileNotFound
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set version = version + 1, updated_at_ms = ? where id = ?`, now, policyID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return findPolicy(policies, policyID)
}

func (s *Service) DeletePolicy(ctx context.Context, policyID string, version int64, confirmation string) error {
	if confirmation != PassthroughConfirmation {
		return ErrPassthroughConfirmation
	}
	now := time.Now().UnixMilli()
	_, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, policyID, "policy_deleted_passthrough", map[string]any{"confirmation": confirmation}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = null where id = ?`, policyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `delete from api_key_policies where id = ?`, policyID); err != nil {
			return err
		}
		return nil
	})
	return err
}

// PurgeOrphaned permanently removes a policy whose upstream key no longer
// exists. The caller must prove orphaned state from the committed config
// snapshot; unlike DeletePolicy this is not a passthrough permission change.
func (s *Service) PurgeOrphaned(ctx context.Context, policyID string, version int64) error {
	now := time.Now().UnixMilli()
	_, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Acquire configuredMu only after the global write barrier and writeMu
		// owned by write(). Takeover activation follows the same lock order.
		s.configuredMu.RLock()
		defer s.configuredMu.RUnlock()
		policies, err := listPolicies(ctx, tx)
		if err != nil {
			return err
		}
		policy, err := findPolicy(policies, policyID)
		if err != nil {
			return err
		}
		if s.configuredHashExists(policy.APIKeyHash) {
			return ErrNotOrphaned
		}
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, policyID, "orphaned_policy_purged", map[string]any{"version": version}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policies set active_profile_id = null where id = ?`, policyID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `delete from api_key_policies where id = ?`, policyID)
		return err
	})
	return err
}

func (s *Service) write(ctx context.Context, operation func(context.Context, *sql.Tx) error) ([]Policy, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	var published []Policy
	err := probackup.Default.ExecuteWrite(ctx, func(ctx context.Context) error {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		tx, err := s.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := operation(ctx, tx); err != nil {
			return err
		}
		policies, err := listPolicies(ctx, tx)
		if err != nil {
			return err
		}
		current := s.index.Load()
		takeoverEnabled := current != nil && current.takeoverEnabled
		next, err := buildRuntimeIndex(policies, takeoverEnabled)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		// The database commit is the durable linearization point. Publishing the
		// already-built immutable index performs no fallible I/O afterward.
		s.publishNextLocked(next)
		published = policies
		return nil
	})
	return published, err
}

func findPolicy(policies []Policy, policyID string) (Policy, error) {
	for _, policy := range policies {
		if policy.ID == policyID {
			return policy, nil
		}
	}
	return Policy{}, ErrPolicyNotFound
}

func randomID(prefix string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}
