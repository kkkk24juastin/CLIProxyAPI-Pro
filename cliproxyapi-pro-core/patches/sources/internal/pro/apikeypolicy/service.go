package apikeypolicy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	Quota           *Quota    `json:"quota,omitempty"`
}

type backupDocument struct {
	SchemaVersion   int                    `json:"schema_version"`
	TakeoverEnabled bool                   `json:"takeover_enabled"`
	Policies        []backupPolicy         `json:"policies"`
	Audits          []AuditRecord          `json:"audits"`
	QuotaAdmissions []backupQuotaAdmission `json:"quota_admissions,omitempty"`
	QuotaEvents     []backupQuotaEvent     `json:"quota_events,omitempty"`
}

type backupQuotaAdmission struct {
	AdmissionID  string `json:"admission_id"`
	PolicyID     string `json:"policy_id"`
	ProfileID    string `json:"profile_id"`
	Epoch        int64  `json:"epoch"`
	AdmittedAtMS int64  `json:"admitted_at_ms"`
}

type backupQuotaEvent struct {
	EventID      string `json:"event_id"`
	AdmissionID  string `json:"admission_id"`
	PolicyID     string `json:"policy_id"`
	ProfileID    string `json:"profile_id"`
	Epoch        int64  `json:"epoch"`
	TotalTokens  int64  `json:"total_tokens"`
	CostMicros   int64  `json:"cost_micros"`
	OccurredAtMS int64  `json:"occurred_at_ms"`
}

func policiesToBackup(policies []Policy) []backupPolicy {
	out := make([]backupPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, backupPolicy{
			ID: policy.ID, APIKeyHash: policy.APIKeyHash, DisplayName: policy.DisplayName,
			ActiveProfileID: policy.ActiveProfileID, Version: policy.Version,
			CreatedAtMS: policy.CreatedAtMS, UpdatedAtMS: policy.UpdatedAtMS,
			Profiles: policy.Profiles, Quota: cloneQuota(policy.Quota),
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
			Profiles: item.Profiles, Quota: cloneQuota(item.Quota),
		})
	}
	return out
}

func (s *Service) ExportBackup(ctx context.Context) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
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
	admissions, events, err := s.listQuotaBackupRecords(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(backupDocument{SchemaVersion: 5, TakeoverEnabled: takeoverEnabled, Policies: policiesToBackup(policies), Audits: audits, QuotaAdmissions: admissions, QuotaEvents: events})
}

func decodeBackup(payload []byte) ([]Policy, []AuditRecord, []backupQuotaAdmission, []backupQuotaEvent, bool, error) {
	var document backupDocument
	if err := json.Unmarshal(payload, &document); err == nil && document.SchemaVersion != 0 {
		if document.SchemaVersion != 2 && document.SchemaVersion != 3 && document.SchemaVersion != 4 && document.SchemaVersion != 5 {
			return nil, nil, nil, nil, false, fmt.Errorf("unsupported API key policy backup schema %d", document.SchemaVersion)
		}
		return backupToPolicies(document.Policies), document.Audits, document.QuotaAdmissions, document.QuotaEvents, document.SchemaVersion >= 3 && document.TakeoverEnabled, nil
	}
	// Version 1 was a bare policy array. It remains importable and contains no
	// audit history by definition.
	var legacy []backupPolicy
	if err := json.Unmarshal(payload, &legacy); err != nil {
		return nil, nil, nil, nil, false, err
	}
	return backupToPolicies(legacy), nil, nil, nil, false, nil
}

func (s *Service) listQuotaBackupRecords(ctx context.Context) ([]backupQuotaAdmission, []backupQuotaEvent, error) {
	admissionRows, err := s.store.db.QueryContext(ctx, `select admission_id, policy_id, profile_id, epoch, admitted_at_ms from api_key_quota_admissions order by admission_id`)
	if err != nil {
		return nil, nil, err
	}
	admissions := make([]backupQuotaAdmission, 0)
	for admissionRows.Next() {
		var item backupQuotaAdmission
		if err := admissionRows.Scan(&item.AdmissionID, &item.PolicyID, &item.ProfileID, &item.Epoch, &item.AdmittedAtMS); err != nil {
			_ = admissionRows.Close()
			return nil, nil, err
		}
		admissions = append(admissions, item)
	}
	if err := admissionRows.Close(); err != nil {
		return nil, nil, err
	}
	eventRows, err := s.store.db.QueryContext(ctx, `select event_id, admission_id, policy_id, profile_id, epoch, total_tokens, cost_micros, occurred_at_ms from api_key_quota_token_events order by event_id`)
	if err != nil {
		return nil, nil, err
	}
	defer eventRows.Close()
	events := make([]backupQuotaEvent, 0)
	for eventRows.Next() {
		var item backupQuotaEvent
		if err := eventRows.Scan(&item.EventID, &item.AdmissionID, &item.PolicyID, &item.ProfileID, &item.Epoch, &item.TotalTokens, &item.CostMicros, &item.OccurredAtMS); err != nil {
			return nil, nil, err
		}
		events = append(events, item)
	}
	return admissions, events, eventRows.Err()
}

func validateQuotaBackupRecords(policies []Policy, admissions []backupQuotaAdmission, events []backupQuotaEvent) error {
	type owner struct {
		policyID  string
		profileID string
		epoch     int64
	}
	quotas := make(map[string]int64)
	for _, policy := range policies {
		if policy.Quota == nil {
			continue
		}
		quotas[policy.ID] = policy.Quota.Epoch
	}
	admissionOwners := make(map[string]owner, len(admissions))
	for _, item := range admissions {
		expectedEpoch, ok := quotas[item.PolicyID]
		if strings.TrimSpace(item.AdmissionID) == "" || strings.TrimSpace(item.ProfileID) == "" || !ok || item.Epoch != expectedEpoch || item.AdmittedAtMS <= 0 {
			return errors.New("invalid API key quota admission backup record")
		}
		if _, duplicate := admissionOwners[item.AdmissionID]; duplicate {
			return errors.New("duplicate API key quota admission backup record")
		}
		admissionOwners[item.AdmissionID] = owner{policyID: item.PolicyID, profileID: item.ProfileID, epoch: item.Epoch}
	}
	seenEvents := make(map[string]struct{}, len(events))
	for _, item := range events {
		expected, ok := admissionOwners[item.AdmissionID]
		if strings.TrimSpace(item.EventID) == "" || !ok || item.PolicyID != expected.policyID || item.Epoch != expected.epoch || item.TotalTokens < 0 || item.CostMicros < 0 || item.OccurredAtMS <= 0 {
			return errors.New("invalid API key quota event backup record")
		}
		if item.ProfileID != expected.profileID {
			return errors.New("invalid API key quota event profile ownership")
		}
		if _, duplicate := seenEvents[item.EventID]; duplicate {
			return errors.New("duplicate API key quota event backup record")
		}
		seenEvents[item.EventID] = struct{}{}
	}
	return nil
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
	policies, _, _, _, targetTakeoverEnabled, _, err := s.stageBackup(payload)
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
	policies, audits, admissions, events, takeoverEnabled, next, err := s.stageBackup(payload)
	if err != nil {
		return err
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
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
		if policy.Quota != nil {
			costLimitMicros, err := quotaCostLimitMicros(policy.Quota.Cost)
			if err != nil {
				return err
			}
			costUsedMicros := int64(math.Round(policy.Quota.Usage.CostUsed * 1_000_000))
			period := normalizeQuotaPeriod(policy.Quota.Period)
			if _, err := tx.ExecContext(ctx, `insert into api_key_policy_quotas(policy_id, enabled, request_limit, token_limit, cost_limit_micros, period_type, period_value, period_unit, epoch, started_at_ms, requests_used, total_tokens_used, cost_used_micros, updated_at_ms) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, policy.ID, policy.Quota.Enabled, policy.Quota.Requests, policy.Quota.TotalTokens, costLimitMicros, period.Type, period.Value, period.Unit, policy.Quota.Epoch, policy.Quota.StartedAtMS, policy.Quota.Usage.RequestsUsed, policy.Quota.Usage.TotalTokensUsed, costUsedMicros, policy.Quota.UpdatedAtMS); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `insert into api_key_quota_generations(policy_id, generation) values(?, ?)`, policy.ID, policy.Quota.Epoch); err != nil {
				return err
			}
		}
	}
	for _, audit := range audits {
		if _, err := tx.ExecContext(ctx, `insert into api_key_policy_audit(id, policy_id, event_type, details_json, created_at_ms) values(?, ?, ?, ?, ?)`, audit.ID, audit.PolicyID, audit.EventType, string(audit.Details), audit.CreatedAtMS); err != nil {
			return err
		}
	}
	for _, admission := range admissions {
		if _, err := tx.ExecContext(ctx, `insert into api_key_quota_admissions(admission_id, policy_id, profile_id, epoch, admitted_at_ms) values(?, ?, ?, ?, ?)`, admission.AdmissionID, admission.PolicyID, admission.ProfileID, admission.Epoch, admission.AdmittedAtMS); err != nil {
			return err
		}
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `insert into api_key_quota_token_events(event_id, admission_id, policy_id, profile_id, epoch, total_tokens, cost_micros, occurred_at_ms) values(?, ?, ?, ?, ?, ?, ?, ?)`, event.EventID, event.AdmissionID, event.PolicyID, event.ProfileID, event.Epoch, event.TotalTokens, event.CostMicros, event.OccurredAtMS); err != nil {
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
		next, err = buildRuntimeIndex(loaded, takeoverEnabled)
		if err != nil {
			return err
		}
		s.publishNextLocked(next)
	} else {
		probackup.AfterCommit(ctx, func() {
			s.writeMu.Lock()
			defer s.writeMu.Unlock()
			if errReload := s.reloadLocked(context.Background()); errReload != nil {
				s.MarkUnavailable()
			}
		})
	}
	return nil
}

// stageBackup is the single canonical boundary shared by preview, runtime-index
// construction and persistence. The returned policies contain exactly the
// normalized values that a successful import will publish and store.
func (s *Service) stageBackup(payload []byte) ([]Policy, []AuditRecord, []backupQuotaAdmission, []backupQuotaEvent, bool, *runtimeIndex, error) {
	policies, audits, admissions, events, takeoverEnabled, err := decodeBackup(payload)
	if err != nil {
		return nil, nil, nil, nil, false, nil, err
	}
	policies, err = s.normalizeBackupPolicies(policies)
	if err != nil {
		return nil, nil, nil, nil, false, nil, err
	}
	if err = validateBackupAudits(audits); err != nil {
		return nil, nil, nil, nil, false, nil, err
	}
	if err = validateQuotaBackupRecords(policies, admissions, events); err != nil {
		return nil, nil, nil, nil, false, nil, err
	}
	next, err := buildRuntimeIndex(policies, takeoverEnabled)
	if err != nil {
		return nil, nil, nil, nil, false, nil, err
	}
	return policies, audits, admissions, events, takeoverEnabled, next, nil
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
		if policy.Quota != nil {
			policy.Quota.Period = normalizeQuotaPeriod(policy.Quota.Period)
			if err := validatePersistedQuota(*policy.Quota); err != nil {
				return nil, fmt.Errorf("policy %q quota: %w", policy.ID, err)
			}
			policy.Quota.Usage = quotaUsage(*policy.Quota)
		}
	}
	return canonical, nil
}

type Service struct {
	store                *Store
	writeMu              sync.Mutex
	quotaMu              sync.Mutex
	catalogMu            sync.RWMutex
	catalogProvider      func() (ProfileCatalog, error)
	configuredMu         sync.RWMutex
	configuredHashes     atomic.Value
	configuredGeneration atomic.Uint64
	index                atomic.Pointer[runtimeIndex]
	retryCtx             context.Context
	retryCancel          context.CancelFunc
	retryMu              sync.Mutex
	retryClosed          bool
	retryWG              sync.WaitGroup
	pendingMu            sync.Mutex
	pendingSettlements   map[string]struct{}
	pendingCount         atomic.Int64
	pricingPending       map[string]struct{}
	pricingBlocked       map[quotaPricingBlockKey]int
	settlementBlocked    map[quotaPricingBlockKey]int
	costEstimatorMu      sync.RWMutex
	costEstimator        func(context.Context, QuotaUsageDelta) (int64, error)
}

type quotaPricingBlockKey struct {
	policyID string
	epoch    int64
}

var errQuotaPricingUnavailable = errors.New("api key quota pricing is unavailable")

// ErrQuotaPriceMissing means the requested model has no active price rule.
// Token usage is still persisted and the event receives a zero-cost quote.
var ErrQuotaPriceMissing = errors.New("api key quota price is missing")

func (s *Service) SetCostEstimator(estimator func(context.Context, QuotaUsageDelta) (int64, error)) {
	if s == nil {
		return
	}
	s.costEstimatorMu.Lock()
	s.costEstimator = estimator
	s.costEstimatorMu.Unlock()
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
	retryCtx, retryCancel := context.WithCancel(context.Background())
	service := &Service{
		store: store, retryCtx: retryCtx, retryCancel: retryCancel,
		pendingSettlements: make(map[string]struct{}), pricingPending: make(map[string]struct{}),
		pricingBlocked: make(map[quotaPricingBlockKey]int), settlementBlocked: make(map[quotaPricingBlockKey]int),
	}
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
	if s.retryCancel != nil {
		s.retryMu.Lock()
		s.retryClosed = true
		s.retryCancel()
		s.retryMu.Unlock()
	}
	s.retryWG.Wait()
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
			Quota:            cloneQuota(policy.Quota),
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

// AdmitDecision reserves one Key-wide request unit immediately before a
// chargeable proxy request enters execution. Discovery and credential-issuing
// routes can keep the same policy decision without consuming the budget.
func (s *Service) AdmitDecision(ctx context.Context, decision RequestPolicyDecision) (RequestPolicyDecision, error) {
	decision = decision.Clone()
	if decision.Mode != ModeProfile || decision.Snapshot == nil || decision.Snapshot.Quota == nil || !decision.Snapshot.Quota.Enabled {
		return decision, nil
	}
	index := s.index.Load()
	if index == nil || !index.healthy {
		return RequestPolicyDecision{}, ErrQuotaUnavailable
	}
	if s.quotaPricingIsBlocked(decision.Snapshot.PolicyID, decision.Snapshot.Quota.Epoch) || s.quotaSettlementIsBlocked(decision.Snapshot.PolicyID, decision.Snapshot.Quota.Epoch) {
		return RequestPolicyDecision{}, ErrQuotaUnavailable
	}
	admissionID, err := s.admitQuota(ctx, *decision.Snapshot)
	if err != nil {
		return RequestPolicyDecision{}, err
	}
	decision.Snapshot.QuotaAdmissionID = admissionID
	attribution, _ := decision.QuotaAttribution()
	requireCost := decision.Snapshot.Quota.Cost != nil
	decision.Snapshot.QuotaUsageSettlement = func(settleCtx context.Context, eventID string, usage QuotaUsageDelta) error {
		if settleCtx == nil {
			settleCtx = context.Background()
		}
		settlementID := attribution.AdmissionID + ":" + eventID
		costMicros, quoted, err := s.recordQuotaUsage(context.WithoutCancel(settleCtx), attribution, settlementID, usage, requireCost)
		if errors.Is(err, ErrQuotaSettlementStale) {
			return nil
		}
		if errors.Is(err, errQuotaPricingUnavailable) {
			s.retryQuotaPricing(attribution, settlementID, usage)
			return err
		}
		if err != nil {
			s.retryQuotaSettlement(attribution, settlementID, usage, requireCost, costMicros, quoted)
		}
		return err
	}
	return decision, nil
}

func validateQuotaInput(input *QuotaInput) error {
	if input == nil {
		return nil
	}
	if input.Requests != nil && *input.Requests <= 0 {
		return errors.New("request quota must be greater than zero")
	}
	if input.TotalTokens != nil && *input.TotalTokens <= 0 {
		return errors.New("token quota must be greater than zero")
	}
	if input.Cost != nil {
		if math.IsNaN(*input.Cost) || math.IsInf(*input.Cost, 0) || *input.Cost <= 0 {
			return errors.New("cost quota must be greater than zero")
		}
		if _, err := usdToMicros(*input.Cost); err != nil {
			return err
		}
	}
	input.Period = normalizeQuotaPeriod(input.Period)
	if err := validateQuotaPeriod(input.Period); err != nil {
		return err
	}
	if input.Enabled && input.Requests == nil && input.TotalTokens == nil && input.Cost == nil {
		return errors.New("enabled quota requires a request, token or cost limit")
	}
	return nil
}

func validatePersistedQuota(quota Quota) error {
	if quota.Epoch <= 0 || quota.StartedAtMS <= 0 || quota.UpdatedAtMS <= 0 || quota.Usage.RequestsUsed < 0 || quota.Usage.TotalTokensUsed < 0 || quota.Usage.CostUsed < 0 {
		return errors.New("invalid persisted quota state")
	}
	return validateQuotaInput(&QuotaInput{Enabled: quota.Enabled, Requests: quota.Requests, TotalTokens: quota.TotalTokens, Cost: quota.Cost, Period: quota.Period})
}

func normalizeQuotaPeriod(period QuotaPeriod) QuotaPeriod {
	period.Type = strings.ToLower(strings.TrimSpace(period.Type))
	period.Unit = strings.ToLower(strings.TrimSpace(period.Unit))
	if period.Type == "" {
		period.Type = QuotaPeriodAllTime
	}
	if period.Type == QuotaPeriodAllTime {
		period.Value = nil
		period.Unit = ""
	}
	if period.Type == QuotaPeriodCalendarDuration {
		period.Value = nil
	}
	return period
}

func validateQuotaPeriod(period QuotaPeriod) error {
	period = normalizeQuotaPeriod(period)
	switch period.Type {
	case QuotaPeriodAllTime:
		return nil
	case QuotaPeriodPastDuration:
		if period.Value == nil || *period.Value <= 0 {
			return errors.New("rolling quota period requires a positive value")
		}
		if period.Unit != "minute" && period.Unit != "hour" && period.Unit != "day" {
			return errors.New("rolling quota period unit must be minute, hour or day")
		}
		if _, ok := quotaPastDuration(*period.Value, period.Unit); !ok {
			return errors.New("rolling quota period is too large")
		}
		return nil
	case QuotaPeriodCalendarDuration:
		if period.Unit != "day" && period.Unit != "month" {
			return errors.New("calendar quota period unit must be day or month")
		}
		return nil
	default:
		return errors.New("quota period type must be all_time, past_duration or calendar_duration")
	}
}

func quotaPastDuration(value int64, unit string) (time.Duration, bool) {
	multiplier := time.Minute
	switch unit {
	case "hour":
		multiplier = time.Hour
	case "day":
		multiplier = 24 * time.Hour
	case "minute":
	default:
		return 0, false
	}
	if value <= 0 || value > int64(math.MaxInt64)/int64(multiplier) {
		return 0, false
	}
	return time.Duration(value) * multiplier, true
}

func quotaPeriodBounds(period QuotaPeriod, startedAtMS, nowMS int64) (int64, int64) {
	period = normalizeQuotaPeriod(period)
	switch period.Type {
	case QuotaPeriodPastDuration:
		if period.Value != nil {
			if duration, ok := quotaPastDuration(*period.Value, period.Unit); ok {
				return nowMS - duration.Milliseconds(), nowMS
			}
		}
	case QuotaPeriodCalendarDuration:
		now := time.UnixMilli(nowMS).UTC()
		if period.Unit == "month" {
			start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			return start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start.UnixMilli(), start.AddDate(0, 0, 1).UnixMilli()
	}
	return startedAtMS, 0
}

func usdToMicros(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > float64(math.MaxInt64)/1_000_000 {
		return 0, errors.New("cost quota is outside the supported range")
	}
	micros := int64(math.Round(value * 1_000_000))
	if micros <= 0 {
		return 0, errors.New("cost quota must be at least 0.000001 USD")
	}
	return micros, nil
}

func mustUSDToMicros(value float64) int64 {
	micros, _ := usdToMicros(value)
	return micros
}

func microsToUSD(value int64) float64 { return float64(value) / 1_000_000 }

func (s *Service) admitQuota(ctx context.Context, snapshot RequestPolicySnapshot) (string, error) {
	if s == nil || s.store == nil || snapshot.Quota == nil || !snapshot.Quota.Enabled {
		return "", nil
	}
	admissionID, err := randomID("quota_request_")
	if err != nil {
		return "", err
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", ErrQuotaUnavailable
	}
	defer tx.Rollback()
	quota, err := getPolicyQuota(ctx, tx, snapshot.PolicyID)
	if err != nil || quota == nil || quota.Epoch != snapshot.Quota.Epoch || !quota.Enabled {
		return "", ErrQuotaUnavailable
	}
	if quota.Requests != nil && quota.Usage.RequestsUsed >= *quota.Requests {
		return "", &QuotaExceededError{Metric: "requests", Used: quota.Usage.RequestsUsed, Limit: *quota.Requests, ResetAtMS: quota.Usage.WindowEndsAtMS}
	}
	if quota.TotalTokens != nil && quota.Usage.TotalTokensUsed >= *quota.TotalTokens {
		return "", &QuotaExceededError{Metric: "total_tokens", Used: quota.Usage.TotalTokensUsed, Limit: *quota.TotalTokens, ResetAtMS: quota.Usage.WindowEndsAtMS}
	}
	if quota.Cost != nil && quota.Usage.CostUsed >= *quota.Cost {
		return "", &QuotaExceededError{Metric: "cost", Used: mustUSDToMicros(quota.Usage.CostUsed), Limit: mustUSDToMicros(*quota.Cost), ResetAtMS: quota.Usage.WindowEndsAtMS}
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `update api_key_policy_quotas set requests_used = requests_used + 1, updated_at_ms = ? where policy_id = ? and epoch = ?`, now, snapshot.PolicyID, quota.Epoch); err != nil {
		return "", ErrQuotaUnavailable
	}
	if _, err = tx.ExecContext(ctx, `insert into api_key_quota_admissions(admission_id, policy_id, profile_id, epoch, admitted_at_ms) values(?, ?, ?, ?, ?)`, admissionID, snapshot.PolicyID, snapshot.ProfileID, quota.Epoch, now); err != nil {
		return "", ErrQuotaUnavailable
	}
	if err = tx.Commit(); err != nil {
		return "", ErrQuotaUnavailable
	}
	return admissionID, nil
}

// RecordQuotaTokens settles one upstream attempt against the Key-wide budget.
// eventID must identify the terminal usage record so retries count once each and
// repeated delivery of the same record remains idempotent.
func (s *Service) RecordQuotaTokens(ctx context.Context, attribution QuotaAttribution, eventID string, totalTokens int64) error {
	_, _, err := s.recordQuotaUsage(ctx, attribution, eventID, QuotaUsageDelta{TotalTokens: totalTokens}, false)
	return err
}

// recordQuotaUsage returns the server-side price quote even when persistence
// fails. Retry callers can then keep the quote stable for this usage event
// instead of re-pricing it against a rule that changed after the request.
func (s *Service) recordQuotaUsage(ctx context.Context, attribution QuotaAttribution, eventID string, usage QuotaUsageDelta, requireCost bool) (costMicros int64, quoted bool, err error) {
	totalTokens := usage.TotalTokens
	if s == nil || s.store == nil || attribution.PolicyID == "" || attribution.AdmissionID == "" || strings.TrimSpace(eventID) == "" || totalTokens < 0 {
		return 0, false, ErrQuotaUnavailable
	}
	if requireCost {
		s.costEstimatorMu.RLock()
		estimator := s.costEstimator
		s.costEstimatorMu.RUnlock()
		if estimator == nil {
			return 0, false, errQuotaPricingUnavailable
		}
		costMicros, err = estimator(ctx, usage)
		if errors.Is(err, ErrQuotaPriceMissing) {
			quoted = true
			return 0, true, s.persistQuotaUsage(ctx, attribution, eventID, totalTokens, 0)
		}
		if err != nil || costMicros < 0 {
			if err == nil {
				err = errors.New("negative cost estimate")
			}
			return 0, false, fmt.Errorf("%w: %v", errQuotaPricingUnavailable, err)
		}
	}
	quoted = true
	err = s.persistQuotaUsage(ctx, attribution, eventID, totalTokens, costMicros)
	return costMicros, quoted, err
}

func (s *Service) persistQuotaUsage(ctx context.Context, attribution QuotaAttribution, eventID string, totalTokens, costMicros int64) error {
	if totalTokens == 0 && costMicros == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, err := s.store.db.BeginTx(dbCtx, nil)
	if err != nil {
		return ErrQuotaUnavailable
	}
	defer tx.Rollback()
	var policyID, profileID string
	var epoch int64
	if err = tx.QueryRowContext(dbCtx, `select policy_id, profile_id, epoch from api_key_quota_admissions where admission_id = ?`, attribution.AdmissionID).Scan(&policyID, &profileID, &epoch); errors.Is(err, sql.ErrNoRows) || err == nil && (policyID != attribution.PolicyID || profileID != attribution.ProfileID || epoch != attribution.Epoch) {
		return fmt.Errorf("%w: %w", ErrQuotaUnavailable, ErrQuotaSettlementStale)
	}
	if err != nil {
		return ErrQuotaUnavailable
	}
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(dbCtx, `insert into api_key_quota_token_events(event_id, admission_id, policy_id, profile_id, epoch, total_tokens, cost_micros, occurred_at_ms) values(?, ?, ?, ?, ?, ?, ?, ?) on conflict(event_id) do nothing`, strings.TrimSpace(eventID), attribution.AdmissionID, attribution.PolicyID, attribution.ProfileID, attribution.Epoch, totalTokens, costMicros, now)
	if err != nil {
		return ErrQuotaUnavailable
	}
	if inserted, _ := result.RowsAffected(); inserted == 0 {
		return nil
	}
	updated, err := tx.ExecContext(dbCtx, `update api_key_policy_quotas set total_tokens_used = total_tokens_used + ?, cost_used_micros = cost_used_micros + ?, updated_at_ms = ? where policy_id = ? and epoch = ?`, totalTokens, costMicros, now, attribution.PolicyID, attribution.Epoch)
	if err != nil {
		return ErrQuotaUnavailable
	}
	if affected, _ := updated.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: %w", ErrQuotaUnavailable, ErrQuotaSettlementStale)
	}
	if err = tx.Commit(); err != nil {
		return ErrQuotaUnavailable
	}
	return nil
}

func (s *Service) retryQuotaSettlement(attribution QuotaAttribution, eventID string, usage QuotaUsageDelta, requireCost bool, costMicros int64, quoted bool) {
	if s == nil || s.retryCtx == nil || usage.empty() {
		return
	}
	s.pendingMu.Lock()
	if _, exists := s.pendingSettlements[eventID]; exists {
		s.pendingMu.Unlock()
		return
	}
	s.pendingSettlements[eventID] = struct{}{}
	s.pendingCount.Add(1)
	s.markQuotaSettlementBlockedLocked(attribution.PolicyID, attribution.Epoch)
	s.pendingMu.Unlock()
	if !s.startQuotaRetry(func() {
		defer s.retryWG.Done()
		defer func() {
			s.pendingMu.Lock()
			delete(s.pendingSettlements, eventID)
			s.pendingCount.Add(-1)
			s.clearQuotaSettlementBlockedLocked(attribution.PolicyID, attribution.Epoch)
			s.pendingMu.Unlock()
		}()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.retryCtx.Done():
				return
			case <-ticker.C:
				var err error
				if quoted {
					err = s.persistQuotaUsage(s.retryCtx, attribution, eventID, usage.TotalTokens, costMicros)
				} else {
					costMicros, quoted, err = s.recordQuotaUsage(s.retryCtx, attribution, eventID, usage, requireCost)
				}
				if err != nil && !errors.Is(err, ErrQuotaSettlementStale) {
					continue
				}
				return
			}
		}
	}) {
		s.pendingMu.Lock()
		delete(s.pendingSettlements, eventID)
		s.pendingCount.Add(-1)
		s.clearQuotaSettlementBlockedLocked(attribution.PolicyID, attribution.Epoch)
		s.pendingMu.Unlock()
	}
}

func (s *Service) startQuotaRetry(retry func()) bool {
	if s == nil || retry == nil {
		return false
	}
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	if s.retryClosed {
		return false
	}
	s.retryWG.Add(1)
	go retry()
	return true
}

func (s *Service) quotaPricingIsBlocked(policyID string, epoch int64) bool {
	if s == nil || policyID == "" || epoch <= 0 {
		return false
	}
	s.pendingMu.Lock()
	blocked := s.pricingBlocked[quotaPricingBlockKey{policyID: policyID, epoch: epoch}] > 0
	s.pendingMu.Unlock()
	return blocked
}

func (s *Service) quotaSettlementIsBlocked(policyID string, epoch int64) bool {
	if s == nil || policyID == "" || epoch <= 0 {
		return false
	}
	s.pendingMu.Lock()
	blocked := s.settlementBlocked[quotaPricingBlockKey{policyID: policyID, epoch: epoch}] > 0
	s.pendingMu.Unlock()
	return blocked
}

func (s *Service) markQuotaSettlementBlockedLocked(policyID string, epoch int64) {
	if policyID != "" && epoch > 0 {
		s.settlementBlocked[quotaPricingBlockKey{policyID: policyID, epoch: epoch}]++
	}
}

func (s *Service) clearQuotaSettlementBlockedLocked(policyID string, epoch int64) {
	key := quotaPricingBlockKey{policyID: policyID, epoch: epoch}
	if remaining := s.settlementBlocked[key] - 1; remaining > 0 {
		s.settlementBlocked[key] = remaining
	} else {
		delete(s.settlementBlocked, key)
	}
}

func (s *Service) retryQuotaPricing(attribution QuotaAttribution, eventID string, usage QuotaUsageDelta) {
	if s == nil || s.retryCtx == nil || usage.empty() || attribution.PolicyID == "" || attribution.Epoch <= 0 {
		return
	}
	blockKey := quotaPricingBlockKey{policyID: attribution.PolicyID, epoch: attribution.Epoch}
	s.pendingMu.Lock()
	if _, exists := s.pricingPending[eventID]; exists {
		s.pendingMu.Unlock()
		return
	}
	s.pricingPending[eventID] = struct{}{}
	s.pricingBlocked[blockKey]++
	s.pendingMu.Unlock()
	if !s.startQuotaRetry(func() {
		defer s.retryWG.Done()
		defer func() {
			s.pendingMu.Lock()
			delete(s.pricingPending, eventID)
			if remaining := s.pricingBlocked[blockKey] - 1; remaining > 0 {
				s.pricingBlocked[blockKey] = remaining
			} else {
				delete(s.pricingBlocked, blockKey)
			}
			s.pendingMu.Unlock()
		}()
		delay := 250 * time.Millisecond
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for {
			select {
			case <-s.retryCtx.Done():
				return
			case <-timer.C:
				costMicros, quoted, err := s.recordQuotaUsage(s.retryCtx, attribution, eventID, usage, true)
				if errors.Is(err, errQuotaPricingUnavailable) {
					if delay < 30*time.Second {
						delay *= 2
						if delay > 30*time.Second {
							delay = 30 * time.Second
						}
					}
					timer.Reset(delay)
					continue
				}
				if err != nil && !errors.Is(err, ErrQuotaSettlementStale) {
					s.retryQuotaSettlement(attribution, eventID, usage, true, costMicros, quoted)
				}
				return
			}
		}
	}) {
		s.pendingMu.Lock()
		delete(s.pricingPending, eventID)
		if remaining := s.pricingBlocked[blockKey] - 1; remaining > 0 {
			s.pricingBlocked[blockKey] = remaining
		} else {
			delete(s.pricingBlocked, blockKey)
		}
		s.pendingMu.Unlock()
	}
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

func (s *Service) Create(ctx context.Context, identity AuthenticatedAPIKeyIdentity, displayName string, initial ProfileInput, quota ...*QuotaInput) (Policy, error) {
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
	var initialQuota *QuotaInput
	if len(quota) > 0 {
		initialQuota = quota[0]
		if err := validateQuotaInput(initialQuota); err != nil {
			return Policy{}, err
		}
	}
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
		if initialQuota != nil {
			if err := replaceQuota(ctx, tx, policyID, initialQuota, now); err != nil {
				return err
			}
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
	if update.Quota.Present {
		if err := validateQuotaInput(update.Quota.Value); err != nil {
			return Policy{}, err
		}
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
		if update.Quota.Present {
			if err := replaceQuota(ctx, tx, policyID, update.Quota.Value, now); err != nil {
				return err
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

func replaceQuota(ctx context.Context, tx *sql.Tx, policyID string, input *QuotaInput, now int64) error {
	if input == nil {
		var existing int
		if err := tx.QueryRowContext(ctx, `select count(*) from api_key_policy_quotas where policy_id = ?`, policyID).Scan(&existing); err != nil || existing == 0 {
			return err
		}
		if _, err := advanceQuotaGeneration(ctx, tx, policyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `delete from api_key_quota_admissions where policy_id = ?`, policyID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `delete from api_key_policy_quotas where policy_id = ?`, policyID)
		return err
	}
	input.Period = normalizeQuotaPeriod(input.Period)
	costLimitMicros, err := quotaCostLimitMicros(input.Cost)
	if err != nil {
		return err
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `select count(*) from api_key_policy_quotas where policy_id = ?`, policyID).Scan(&existing); err != nil {
		return err
	}
	if existing == 0 {
		epoch, err := advanceQuotaGeneration(ctx, tx, policyID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `insert into api_key_policy_quotas(policy_id, enabled, request_limit, token_limit, cost_limit_micros, period_type, period_value, period_unit, epoch, started_at_ms, requests_used, total_tokens_used, cost_used_micros, updated_at_ms) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?)`, policyID, input.Enabled, input.Requests, input.TotalTokens, costLimitMicros, input.Period.Type, input.Period.Value, input.Period.Unit, epoch, now, now)
		return err
	}
	var currentCost, currentPeriodValue sql.NullInt64
	var currentPeriodType, currentPeriodUnit string
	if err := tx.QueryRowContext(ctx, `select cost_limit_micros, period_type, period_value, period_unit from api_key_policy_quotas where policy_id = ?`, policyID).Scan(&currentCost, &currentPeriodType, &currentPeriodValue, &currentPeriodUnit); err != nil {
		return err
	}
	resetUsage := !nullableInt64Equals(currentCost, costLimitMicros) || currentPeriodType != input.Period.Type || !nullableInt64Equals(currentPeriodValue, input.Period.Value) || currentPeriodUnit != input.Period.Unit
	if resetUsage {
		epoch, err := advanceQuotaGeneration(ctx, tx, policyID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `delete from api_key_quota_admissions where policy_id = ?`, policyID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `update api_key_policy_quotas set enabled = ?, request_limit = ?, token_limit = ?, cost_limit_micros = ?, period_type = ?, period_value = ?, period_unit = ?, epoch = ?, started_at_ms = ?, requests_used = 0, total_tokens_used = 0, cost_used_micros = 0, updated_at_ms = ? where policy_id = ?`, input.Enabled, input.Requests, input.TotalTokens, costLimitMicros, input.Period.Type, input.Period.Value, input.Period.Unit, epoch, now, now, policyID)
		return err
	}
	_, err = tx.ExecContext(ctx, `update api_key_policy_quotas set enabled = ?, request_limit = ?, token_limit = ?, cost_limit_micros = ?, period_type = ?, period_value = ?, period_unit = ?, updated_at_ms = ? where policy_id = ?`, input.Enabled, input.Requests, input.TotalTokens, costLimitMicros, input.Period.Type, input.Period.Value, input.Period.Unit, now, policyID)
	return err
}

func quotaCostLimitMicros(cost *float64) (any, error) {
	if cost == nil {
		return nil, nil
	}
	return usdToMicros(*cost)
}

func nullableInt64Equals(current sql.NullInt64, next any) bool {
	if next == nil {
		return !current.Valid
	}
	value, ok := next.(int64)
	if pointer, pointerOK := next.(*int64); pointerOK {
		if pointer == nil {
			return !current.Valid
		}
		value, ok = *pointer, true
	}
	return ok && current.Valid && current.Int64 == value
}

// advanceQuotaGeneration keeps the monotonic budget identity outside the
// optional quota row. This lets quota deletion clean all historical request
// rows without allowing a later recreation to reuse an old in-flight epoch.
func advanceQuotaGeneration(ctx context.Context, tx *sql.Tx, policyID string) (int64, error) {
	var generation int64
	if err := tx.QueryRowContext(ctx, `insert into api_key_quota_generations(policy_id, generation)
		select ?, max(generation) + 1 from (
			select 0 as generation
			union all select epoch from api_key_policy_quotas where policy_id = ?
			union all select epoch from api_key_quota_admissions where policy_id = ?
		) where true on conflict(policy_id) do update set generation = max(api_key_quota_generations.generation + 1, excluded.generation)
		returning generation`, policyID, policyID, policyID).Scan(&generation); err != nil {
		return 0, err
	}
	return generation, nil
}

func (s *Service) ResetQuota(ctx context.Context, policyID string, version int64, confirmation string) (Policy, error) {
	if confirmation != QuotaResetConfirmation {
		return Policy{}, ErrQuotaResetConfirmation
	}
	now := time.Now().UnixMilli()
	policies, err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireVersion(ctx, tx, policyID, version); err != nil {
			return err
		}
		quota, err := getPolicyQuota(ctx, tx, policyID)
		if err != nil {
			return err
		}
		if quota == nil {
			return ErrQuotaNotConfigured
		}
		nextEpoch, err := advanceQuotaGeneration(ctx, tx, policyID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `delete from api_key_quota_admissions where policy_id = ?`, policyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update api_key_policy_quotas set epoch = ?, started_at_ms = ?, requests_used = 0, total_tokens_used = 0, cost_used_micros = 0, updated_at_ms = ? where policy_id = ?`, nextEpoch, now, now, policyID); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, policyID, "api_key_quota_reset", map[string]any{
			"previousEpoch": quota.Epoch, "previousRequestsUsed": quota.Usage.RequestsUsed,
			"previousTotalTokensUsed": quota.Usage.TotalTokensUsed, "confirmation": confirmation,
			"previousCostUsed": quota.Usage.CostUsed,
		}, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `update api_key_policies set version = version + 1, updated_at_ms = ? where id = ?`, now, policyID)
		return err
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
		s.quotaMu.Lock()
		defer s.quotaMu.Unlock()
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
