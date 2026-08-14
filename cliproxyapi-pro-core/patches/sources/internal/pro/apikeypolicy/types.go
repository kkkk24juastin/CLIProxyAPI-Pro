// Package apikeypolicy owns API-key policy identity, persistence, immutable
// request decisions and Management-facing domain operations.
package apikeypolicy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

const (
	ModePassthrough = "passthrough"
	ModeProfile     = "profile"

	StateUnconfigured = "unconfigured"
	StateConfigured   = "configured"
	StateOrphaned     = "orphaned"
	StateUnavailable  = "unavailable"
)

var (
	ErrUnavailable             = errors.New("api key policy unavailable")
	ErrTakeoverStateChanged    = errors.New("api key policy takeover state changed")
	ErrPolicyNotFound          = errors.New("api key policy not found")
	ErrProfileNotFound         = errors.New("api key profile not found")
	ErrVersionConflict         = errors.New("api key policy version conflict")
	ErrActiveProfileDelete     = errors.New("active api key profile cannot be deleted")
	ErrLastProfileDelete       = errors.New("last api key profile cannot be deleted")
	ErrPassthroughConfirmation = errors.New("policy deletion requires unrestricted passthrough confirmation")
	ErrOrphaned                = errors.New("api key policy is orphaned")
	ErrNotOrphaned             = errors.New("api key policy belongs to a configured upstream key")
)

const PassthroughConfirmation = "RESTORE_UNRESTRICTED_PASSTHROUGH"

// AuthenticatedAPIKeyIdentity can only be constructed from a successful
// upstream access result. Raw keys never leave this constructor.
type AuthenticatedAPIKeyIdentity struct {
	hash string
}

func NewAuthenticatedAPIKeyIdentity(raw string) (AuthenticatedAPIKeyIdentity, error) {
	if raw == "" {
		return AuthenticatedAPIKeyIdentity{}, errors.New("authenticated api key is empty")
	}
	sum := sha256.Sum256([]byte(raw))
	return AuthenticatedAPIKeyIdentity{hash: hex.EncodeToString(sum[:])}, nil
}

func (i AuthenticatedAPIKeyIdentity) Hash() string { return i.hash }
func (i AuthenticatedAPIKeyIdentity) Valid() bool  { return len(i.hash) == sha256.Size*2 }

type identityContextKey struct{}
type decisionContextKey struct{}

func WithIdentity(ctx context.Context, identity AuthenticatedAPIKeyIdentity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !identity.Valid() {
		return ctx
	}
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (AuthenticatedAPIKeyIdentity, bool) {
	if ctx == nil {
		return AuthenticatedAPIKeyIdentity{}, false
	}
	identity, ok := ctx.Value(identityContextKey{}).(AuthenticatedAPIKeyIdentity)
	return identity, ok && identity.Valid()
}

// InheritContext copies only server-issued API-key policy values from source.
// It is used when handlers intentionally create a new execution context.
func InheritContext(destination, source context.Context) context.Context {
	if destination == nil {
		destination = context.Background()
	}
	if identity, ok := IdentityFromContext(source); ok {
		destination = WithIdentity(destination, identity)
	}
	if decision, ok := DecisionFromContext(source); ok {
		destination = WithDecision(destination, decision)
	}
	return destination
}

func WithDecision(ctx context.Context, decision RequestPolicyDecision) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, decisionContextKey{}, decision.Clone())
}

func DecisionFromContext(ctx context.Context) (RequestPolicyDecision, bool) {
	if ctx == nil {
		return RequestPolicyDecision{}, false
	}
	decision, ok := ctx.Value(decisionContextKey{}).(RequestPolicyDecision)
	return decision.Clone(), ok
}

type ModelMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type Profile struct {
	ID          string         `json:"id"`
	PolicyID    string         `json:"policyId"`
	Name        string         `json:"name"`
	Providers   []string       `json:"providers"`
	Models      []string       `json:"models"`
	Mappings    []ModelMapping `json:"mappings"`
	CreatedAtMS int64          `json:"createdAtMs"`
	UpdatedAtMS int64          `json:"updatedAtMs"`
}

type Policy struct {
	ID              string    `json:"id"`
	APIKeyHash      string    `json:"-"`
	DisplayName     string    `json:"displayName"`
	ActiveProfileID string    `json:"activeProfileId"`
	Version         int64     `json:"version"`
	CreatedAtMS     int64     `json:"createdAtMs"`
	UpdatedAtMS     int64     `json:"updatedAtMs"`
	Profiles        []Profile `json:"profiles"`
	State           string    `json:"state,omitempty"`
}

// AuditRecord is retained across policy backups. Fingerprints and raw keys are
// intentionally absent; policy_id may refer to a policy that was deleted by
// the audited security operation.
type AuditRecord struct {
	ID          int64           `json:"id"`
	PolicyID    string          `json:"policyId"`
	EventType   string          `json:"eventType"`
	Details     json.RawMessage `json:"details"`
	CreatedAtMS int64           `json:"createdAtMs"`
}

type ProfileInput struct {
	Name      string         `json:"name"`
	Providers []string       `json:"providers"`
	Models    []string       `json:"models"`
	Mappings  []ModelMapping `json:"mappings"`
}

// WorkspaceUpdate is the atomic Management write unit for one open policy
// workspace. ProfileID selects an existing profile; CreateProfile requests a
// new inactive profile. A nil Profile updates only the display name.
type WorkspaceUpdate struct {
	DisplayName   string
	ProfileID     string
	Profile       *ProfileInput
	CreateProfile bool
}

// ProfileCatalog is the server-authoritative set of provider and model IDs
// accepted by new Profile writes. The runtime registry may change between
// calls, so callers obtain a fresh catalog for every write.
type ProfileCatalog struct {
	Providers []string `json:"providers"`
	Models    []string `json:"models"`
}

func NewProfileCatalog(providers, models []string) ProfileCatalog {
	return ProfileCatalog{
		Providers: normalizeUnique(providers, normalizeProvider),
		Models:    normalizeUnique(models, normalizeModel),
	}
}

func (c ProfileCatalog) Validate(input ProfileInput) error {
	input, err := normalizeProfileInput(input)
	if err != nil {
		return err
	}
	providers := make(map[string]struct{}, len(c.Providers))
	for _, provider := range normalizeUnique(c.Providers, normalizeProvider) {
		providers[provider] = struct{}{}
	}
	models := make(map[string]struct{}, len(c.Models))
	for _, model := range normalizeUnique(c.Models, normalizeModel) {
		models[model] = struct{}{}
	}
	for _, provider := range input.Providers {
		if _, ok := providers[provider]; !ok {
			return fmt.Errorf("unknown provider %q", provider)
		}
	}
	for _, model := range input.Models {
		if _, ok := models[model]; !ok {
			return fmt.Errorf("unknown model %q", model)
		}
	}
	return nil
}

type RequestPolicySnapshot struct {
	PolicyID         string
	APIKeyHash       string
	ProfileID        string
	ProfileName      string
	Version          int64
	ModelMappings    map[string]string
	AllowedModels    map[string]struct{}
	AllowedProviders map[string]struct{}
	RequestedModel   string
	EffectiveModel   string
}

func (s RequestPolicySnapshot) Clone() RequestPolicySnapshot {
	s.ModelMappings = cloneStringMap(s.ModelMappings)
	s.AllowedModels = cloneStringSet(s.AllowedModels)
	s.AllowedProviders = cloneStringSet(s.AllowedProviders)
	return s
}

type RequestPolicyDecision struct {
	Mode     string
	Snapshot *RequestPolicySnapshot
}

// UsageAttribution is copied from the immutable request decision by usage
// sinks. It never contains a raw API key.
type UsageAttribution struct {
	PolicyMode     string
	APIKeyPolicyID string
	ProfileID      string
	ProfileName    string
	RequestedModel string
	EffectiveModel string
}

func (d RequestPolicyDecision) UsageAttribution() UsageAttribution {
	attribution := UsageAttribution{PolicyMode: d.Mode}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return attribution
	}
	attribution.APIKeyPolicyID = d.Snapshot.PolicyID
	attribution.ProfileID = d.Snapshot.ProfileID
	attribution.ProfileName = d.Snapshot.ProfileName
	attribution.RequestedModel = d.Snapshot.RequestedModel
	attribution.EffectiveModel = d.Snapshot.EffectiveModel
	return attribution
}

type ModelCandidate struct {
	ID        string
	Providers []string
}

type VisibleModel struct {
	ID          string
	EffectiveID string
}

func PassthroughDecision() RequestPolicyDecision {
	return RequestPolicyDecision{Mode: ModePassthrough}
}

func (d RequestPolicyDecision) Clone() RequestPolicyDecision {
	if d.Snapshot != nil {
		cloned := d.Snapshot.Clone()
		d.Snapshot = &cloned
	}
	return d
}

func (d RequestPolicyDecision) ApplyModel(requested string) (string, error) {
	if d.Mode == ModePassthrough {
		return requested, nil
	}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return "", ErrUnavailable
	}
	return d.applyProfileModel(requested, true)
}

// HasExactModelMapping reports whether the active Profile owns the precise
// client-facing model (before auto or thinking-suffix normalization).
func (d RequestPolicyDecision) HasExactModelMapping(model string) bool {
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return false
	}
	_, ok := d.Snapshot.ModelMappings[normalizeModel(model)]
	return ok
}

// ValidateEffectiveModel checks a model selected after the initial profile
// mapping (for example, a ModelRouter target). It deliberately does not apply
// mappings a second time: a router target is an execution model, not another
// client alias.
func (d RequestPolicyDecision) ValidateEffectiveModel(model string) (string, error) {
	if d.Mode == ModePassthrough {
		return model, nil
	}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return "", ErrUnavailable
	}
	return d.applyProfileModel(model, false)
}

func (d RequestPolicyDecision) applyProfileModel(model string, applyMapping bool) (string, error) {
	model = normalizeModel(model)
	if model == "" {
		return "", profileModelForbiddenError()
	}

	// Exact registered models (including custom IDs ending in parentheses) and
	// exact aliases win before a trailing segment is interpreted as thinking.
	if applyMapping {
		if mapped := normalizeModel(d.Snapshot.ModelMappings[model]); mapped != "" {
			if _, allowed := d.Snapshot.AllowedModels[mapped]; !allowed {
				return "", profileModelForbiddenError()
			}
			return mapped, nil
		}
	}
	if _, allowed := d.Snapshot.AllowedModels[model]; allowed {
		return model, nil
	}

	parsed := thinking.ParseSuffix(model)
	baseModel := normalizeModel(parsed.ModelName)
	if !parsed.HasSuffix || baseModel == "" || baseModel == model {
		return "", profileModelForbiddenError()
	}
	effectiveBase := baseModel
	if applyMapping {
		if mapped := normalizeModel(d.Snapshot.ModelMappings[baseModel]); mapped != "" {
			effectiveBase = mapped
		}
	}
	if _, allowed := d.Snapshot.AllowedModels[effectiveBase]; !allowed {
		return "", profileModelForbiddenError()
	}
	return effectiveBase + "(" + parsed.RawSuffix + ")", nil
}

func profileModelForbiddenError() error {
	return &PolicyError{Code: "profile_model_forbidden", Message: "model is not allowed by the active API key profile"}
}

// WithModels records the client-requested and policy-effective model on a
// cloned decision. The returned value is safe to attach to one request.
func (d RequestPolicyDecision) WithModels(requested, effective string) RequestPolicyDecision {
	d = d.Clone()
	if d.Snapshot != nil {
		d.Snapshot.RequestedModel = strings.TrimSpace(requested)
		d.Snapshot.EffectiveModel = strings.TrimSpace(effective)
	}
	return d
}

func (d RequestPolicyDecision) AllowsProvider(provider string) error {
	if d.Mode == ModePassthrough {
		return nil
	}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return ErrUnavailable
	}
	provider = normalizeProvider(provider)
	if provider == "" {
		return &PolicyError{Code: "profile_provider_forbidden", Message: "execution provider cannot be resolved for the active API key profile"}
	}
	if _, allowed := d.Snapshot.AllowedProviders[provider]; !allowed {
		return &PolicyError{Code: "profile_provider_forbidden", Message: "provider is not allowed by the active API key profile"}
	}
	return nil
}

func (d RequestPolicyDecision) FilterProviders(providers []string) ([]string, error) {
	if d.Mode == ModePassthrough {
		return append([]string(nil), providers...), nil
	}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return nil, ErrUnavailable
	}
	out := make([]string, 0, len(providers))
	seen := make(map[string]struct{})
	for _, provider := range providers {
		provider = normalizeProvider(provider)
		if provider == "" {
			continue
		}
		if _, allowed := d.Snapshot.AllowedProviders[provider]; !allowed {
			continue
		}
		if _, duplicate := seen[provider]; duplicate {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	if len(out) == 0 {
		return nil, &PolicyError{Code: "profile_provider_forbidden", Message: "no provider is allowed by the active API key profile"}
	}
	return out, nil
}

// FilterVisibleModels returns canonical allowed models plus exact mapping
// aliases whose targets are currently carried by at least one allowed provider.
func (d RequestPolicyDecision) FilterVisibleModels(candidates []ModelCandidate) ([]VisibleModel, error) {
	if d.Mode == ModePassthrough {
		out := make([]VisibleModel, 0, len(candidates))
		for _, candidate := range candidates {
			if id := normalizeModel(candidate.ID); id != "" {
				out = append(out, VisibleModel{ID: id, EffectiveID: id})
			}
		}
		return out, nil
	}
	if d.Mode != ModeProfile || d.Snapshot == nil {
		return nil, ErrUnavailable
	}
	available := make(map[string]ModelCandidate, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.ID = normalizeModel(candidate.ID)
		if candidate.ID == "" {
			continue
		}
		if _, exists := available[candidate.ID]; !exists {
			order = append(order, candidate.ID)
		}
		available[candidate.ID] = candidate
	}
	providerVisible := func(candidate ModelCandidate) bool {
		for _, provider := range candidate.Providers {
			if _, allowed := d.Snapshot.AllowedProviders[normalizeProvider(provider)]; allowed {
				return true
			}
		}
		return false
	}
	out := make([]VisibleModel, 0, len(order)+len(d.Snapshot.ModelMappings))
	for _, id := range order {
		candidate := available[id]
		if _, allowed := d.Snapshot.AllowedModels[id]; allowed && providerVisible(candidate) {
			out = append(out, VisibleModel{ID: id, EffectiveID: id})
		}
	}
	aliases := make([]string, 0, len(d.Snapshot.ModelMappings))
	for alias := range d.Snapshot.ModelMappings {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		target := d.Snapshot.ModelMappings[alias]
		candidate, exists := available[target]
		if exists && providerVisible(candidate) {
			out = append(out, VisibleModel{ID: alias, EffectiveID: target})
		}
	}
	return out, nil
}

type PolicyError struct {
	Code    string
	Message string
}

func (e *PolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func normalizeProfileInput(input ProfileInput) (ProfileInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return ProfileInput{}, errors.New("profile name is required")
	}
	input.Providers = normalizeUnique(input.Providers, normalizeProvider)
	input.Models = normalizeUnique(input.Models, normalizeModel)
	if len(input.Providers) == 0 {
		return ProfileInput{}, errors.New("at least one provider is required")
	}
	if len(input.Models) == 0 {
		return ProfileInput{}, errors.New("at least one model is required")
	}
	allowedModels := make(map[string]struct{}, len(input.Models))
	for _, model := range input.Models {
		allowedModels[model] = struct{}{}
	}
	mappings := make([]ModelMapping, 0, len(input.Mappings))
	sources := make(map[string]struct{}, len(input.Mappings))
	for _, mapping := range input.Mappings {
		mapping.Source = normalizeModel(mapping.Source)
		mapping.Target = normalizeModel(mapping.Target)
		if mapping.Source == "" || mapping.Target == "" || mapping.Source == mapping.Target {
			return ProfileInput{}, errors.New("model mapping requires distinct source and target models")
		}
		if _, duplicate := sources[mapping.Source]; duplicate {
			return ProfileInput{}, errors.New("model mapping source must be unique")
		}
		if _, allowed := allowedModels[mapping.Target]; !allowed {
			return ProfileInput{}, errors.New("model mapping target must be allowed")
		}
		sources[mapping.Source] = struct{}{}
		mappings = append(mappings, mapping)
	}
	for _, mapping := range mappings {
		if _, chained := sources[mapping.Target]; chained {
			return ProfileInput{}, errors.New("chained model mappings are not allowed")
		}
	}
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].Source < mappings[j].Source })
	input.Mappings = mappings
	return input, nil
}

func normalizeProvider(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeModel(value string) string    { return strings.TrimSpace(value) }

func normalizeUnique(values []string, normalize func(string) string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalize(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for value := range in {
		out[value] = struct{}{}
	}
	return out
}
