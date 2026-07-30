package inspection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	embeddedusage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage"
)

type AuthStatus string

const (
	AuthStatusActive       AuthStatus = "active"
	AuthStatusDisabled     AuthStatus = "disabled"
	AuthStatusError        AuthStatus = "error"
	AttributeVirtualSource            = "virtual_source"
)

type AuthError struct {
	Code       string
	Message    string
	HTTPStatus int
	Retryable  bool
}

type QuotaSnapshot struct {
	SchemaVersion int         `json:"schema_version"`
	Provider      string      `json:"provider"`
	ObservedAtMS  int64       `json:"observed_at_ms"`
	Items         []QuotaItem `json:"items"`
}

type QuotaItem struct {
	UsedPercent       *float64 `json:"used_percent,omitempty"`
	RemainingFraction *float64 `json:"remaining_fraction,omitempty"`
}

// Auth is an inspection-only, non-secret credential projection. It mirrors the
// legacy scheduler's read model without importing Core auth internals.
type Auth struct {
	ID               string
	Index            string
	Provider         string
	Label            string
	FileName         string
	Status           AuthStatus
	StatusMessage    string
	Disabled         bool
	Unavailable      bool
	UpdatedAt        time.Time
	NextRefreshAfter time.Time
	Metadata         map[string]any
	Attributes       map[string]string
	LastError        *AuthError
}

func (a *Auth) EnsureIndex() {
	if a != nil && strings.TrimSpace(a.Index) == "" {
		a.Index = strings.TrimSpace(a.ID)
	}
}

func (a *Auth) Clone() *Auth {
	if a == nil {
		return nil
	}
	clone := *a
	clone.Metadata = cloneAnyMap(a.Metadata)
	clone.Attributes = make(map[string]string, len(a.Attributes))
	for key, value := range a.Attributes {
		clone.Attributes[key] = value
	}
	if a.LastError != nil {
		lastError := *a.LastError
		clone.LastError = &lastError
	}
	return &clone
}

func isPluginVirtualAuth(auth *Auth) bool {
	return auth != nil && strings.TrimSpace(authAttribute(auth, AttributeVirtualSource)) != ""
}

func clearRoutingProtectionOwnership(auth *Auth) {
	if auth != nil && auth.Metadata != nil {
		delete(auth.Metadata, routingProtectionMetadataKey)
	}
}

// HostAuthEntry is the non-secret inspection view returned by Core. Credential
// material never crosses the plugin boundary; authenticated HTTP remains a
// host operation pinned to AuthIndex.
type HostAuthEntry struct {
	ID                 string            `json:"id"`
	AuthIndex          string            `json:"auth_index"`
	Name               string            `json:"name"`
	Type               string            `json:"type"`
	Provider           string            `json:"provider"`
	Label              string            `json:"label,omitempty"`
	Email              string            `json:"email,omitempty"`
	Status             string            `json:"status,omitempty"`
	StatusMessage      string            `json:"status_message,omitempty"`
	Disabled           bool              `json:"disabled,omitempty"`
	Unavailable        bool              `json:"unavailable,omitempty"`
	RuntimeOnly        bool              `json:"runtime_only,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at,omitempty"`
	Revision           int64             `json:"revision,omitempty"`
	NextRefreshAfter   time.Time         `json:"next_refresh_after,omitempty"`
	DisplayName        string            `json:"display_name,omitempty"`
	AccountID          string            `json:"account_id,omitempty"`
	UserID             string            `json:"user_id,omitempty"`
	PlanType           string            `json:"plan_type,omitempty"`
	UsingAPI           *bool             `json:"using_api,omitempty"`
	VirtualSource      string            `json:"virtual_source,omitempty"`
	InspectionMetadata map[string]any    `json:"inspection_metadata,omitempty"`
	InspectionAttrs    map[string]string `json:"inspection_attributes,omitempty"`
}

type HostAuthRefreshResponse struct {
	Triggered bool          `json:"triggered"`
	Refreshed bool          `json:"refreshed"`
	Auth      HostAuthEntry `json:"auth"`
}

type HostHTTPRequest struct {
	AuthIndex        string      `json:"auth_index"`
	Method           string      `json:"method"`
	URL              string      `json:"url"`
	Headers          http.Header `json:"headers,omitempty"`
	Body             []byte      `json:"body,omitempty"`
	TimeoutMS        int         `json:"timeout_ms,omitempty"`
	MaxResponseBytes int64       `json:"max_response_bytes,omitempty"`
}

type HostHTTPResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"body,omitempty"`
	Truncated  bool        `json:"truncated,omitempty"`
}

type HostHealthError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
}

type HostHealthPatch struct {
	AuthIndex        string           `json:"auth_index"`
	ExpectedRevision int64            `json:"expected_revision,omitempty"`
	Disabled         *bool            `json:"disabled,omitempty"`
	Error            *HostHealthError `json:"error,omitempty"`
	ClearError       bool             `json:"clear_error,omitempty"`
}

type HostQuotaResponse struct {
	Snapshot       json.RawMessage `json:"snapshot"`
	UpstreamStatus int             `json:"upstream_status,omitempty"`
	Handled        bool            `json:"handled"`
	Error          string          `json:"error,omitempty"`
}

// Gateway is the complete privileged surface needed by account inspection.
// It is deliberately narrower than the host auth manager.
type Gateway interface {
	List(context.Context) ([]HostAuthEntry, error)
	Refresh(context.Context, string, bool) (HostAuthRefreshResponse, error)
	HTTPDo(context.Context, HostHTTPRequest) (HostHTTPResponse, error)
	PatchHealth(context.Context, HostHealthPatch) (HostAuthEntry, error)
	Delete(context.Context, string, int64) (string, error)
	FetchQuota(context.Context, string) (HostQuotaResponse, error)
}

type compatHandler struct {
	gateway          Gateway
	authManager      *compatAuthManager
	lifecycleContext context.Context
	config           Config
}

type Config struct {
	CodexUserAgent  string
	ClaudeUserAgent string
}

func newCompatHandler(ctx context.Context, gateway Gateway, config Config) *compatHandler {
	if ctx == nil {
		ctx = context.Background()
	}
	h := &compatHandler{gateway: gateway, lifecycleContext: ctx, config: config}
	h.authManager = &compatAuthManager{gateway: gateway}
	return h
}

func (h *compatHandler) authByIndex(index string) *Auth {
	if h == nil || h.authManager == nil {
		return nil
	}
	for _, auth := range h.authManager.List() {
		if auth != nil && auth.Index == strings.TrimSpace(index) {
			return auth
		}
	}
	return nil
}

func (h *compatHandler) fetchAndPersistPluginQuota(ctx context.Context, auth *Auth) (hostQuotaResult, int, string, error) {
	if h == nil || h.gateway == nil || auth == nil {
		return hostQuotaResult{}, http.StatusServiceUnavailable, "plugin quota service unavailable", fmt.Errorf("plugin quota service unavailable")
	}
	response, err := h.gateway.FetchQuota(ctx, auth.Index)
	result := hostQuotaResult{UpstreamStatus: response.UpstreamStatus}
	if len(response.Snapshot) > 0 {
		if decodeErr := json.Unmarshal(response.Snapshot, &result.Snapshot); decodeErr != nil {
			return result, http.StatusBadGateway, "quota fetch failed", fmt.Errorf("decode quota snapshot: %w", decodeErr)
		}
	}
	if err != nil {
		return result, http.StatusBadGateway, "quota fetch failed", err
	}
	if !response.Handled {
		return result, http.StatusNotFound, "quota provider not found", fmt.Errorf("quota provider not found")
	}
	if response.Error != "" {
		return result, http.StatusBadGateway, "quota fetch failed", fmt.Errorf("%s", response.Error)
	}
	if persistErr := persistHostQuotaSnapshot(ctx, auth, response.Snapshot, result.Snapshot); persistErr != nil {
		return result, http.StatusInternalServerError, "quota persistence failed", persistErr
	}
	return result, http.StatusOK, "", nil
}

type hostQuotaResult struct {
	Snapshot       QuotaSnapshot
	UpstreamStatus int
}

func persistHostQuotaSnapshot(ctx context.Context, auth *Auth, raw json.RawMessage, snapshot QuotaSnapshot) error {
	if auth == nil || len(raw) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	observed := snapshot.ObservedAtMS
	if observed <= 0 {
		observed = now
	}
	fingerprint := sha256.Sum256([]byte(strings.ToLower(auth.Provider + "|" + auth.Index)))
	return embeddedusage.SetQuotaCache(ctx, embeddedusage.QuotaCacheEntry{
		ID: "quota-provider:" + auth.Provider + ":" + auth.Index, Provider: auth.Provider,
		FileName: auth.FileName, AuthIndex: auth.Index, IdentityFingerprint: hex.EncodeToString(fingerprint[:]),
		Data: append(json.RawMessage(nil), raw...), CachedAt: observed, ObservedAt: observed, AccessedAt: now,
		Version: snapshot.SchemaVersion,
	})
}

func (h *compatHandler) deleteAuthFileByName(ctx context.Context, name string) (string, bool, error) {
	auth := h.authByFileName(name)
	if auth == nil {
		return "", false, fmt.Errorf("auth not found")
	}
	deleted, err := h.gateway.Delete(ctx, auth.Index, authRevisionOf(auth))
	return deleted, err == nil, err
}

func (h *compatHandler) authByFileName(name string) *Auth {
	if h == nil || h.authManager == nil {
		return nil
	}
	for _, auth := range h.authManager.List() {
		if auth != nil && strings.EqualFold(strings.TrimSpace(auth.FileName), strings.TrimSpace(name)) {
			return auth
		}
	}
	return nil
}

type compatAuthManager struct {
	gateway Gateway
}

func (m *compatAuthManager) List() []*Auth {
	if m == nil || m.gateway == nil {
		return nil
	}
	entries, err := m.gateway.List(context.Background())
	if err != nil {
		return nil
	}
	auths := make([]*Auth, 0, len(entries))
	for _, entry := range entries {
		auths = append(auths, authFromHostEntry(entry))
	}
	return auths
}

func (m *compatAuthManager) RefreshIfDueForInspection(ctx context.Context, id string) (*Auth, bool, error) {
	return m.refresh(ctx, id, false)
}

func (m *compatAuthManager) ForceRefreshForInspection(ctx context.Context, id string) (*Auth, bool, error) {
	return m.refresh(ctx, id, true)
}

func (m *compatAuthManager) refresh(ctx context.Context, id string, force bool) (*Auth, bool, error) {
	for _, auth := range m.List() {
		if auth != nil && auth.ID == id {
			response, err := m.gateway.Refresh(ctx, auth.Index, force)
			if err != nil {
				return nil, false, err
			}
			return authFromHostEntry(response.Auth), response.Refreshed, nil
		}
	}
	return nil, false, fmt.Errorf("auth not found")
}

func (m *compatAuthManager) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error) {
	if m == nil || m.gateway == nil || auth == nil || req == nil {
		return nil, fmt.Errorf("auth HTTP gateway unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	response, err := m.gateway.HTTPDo(ctx, HostHTTPRequest{
		AuthIndex: auth.Index, Method: req.Method, URL: req.URL.String(), Headers: req.Header.Clone(), Body: body,
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: response.StatusCode, Header: response.Headers.Clone(), Body: io.NopCloser(strings.NewReader(string(response.Body))),
	}, nil
}

func (m *compatAuthManager) Update(ctx context.Context, auth *Auth) (*Auth, error) {
	if m == nil || m.gateway == nil || auth == nil {
		return nil, fmt.Errorf("auth health gateway unavailable")
	}
	disabled := auth.Disabled
	patch := HostHealthPatch{AuthIndex: auth.Index, ExpectedRevision: authRevisionOf(auth), Disabled: &disabled}
	if auth.LastError == nil {
		patch.ClearError = true
	} else {
		patch.Error = &HostHealthError{Code: auth.LastError.Code, Message: auth.LastError.Message, HTTPStatus: auth.LastError.HTTPStatus}
	}
	updated, err := m.gateway.PatchHealth(ctx, patch)
	if err != nil {
		return nil, err
	}
	return authFromHostEntry(updated), nil
}

func authFromHostEntry(entry HostAuthEntry) *Auth {
	metadata := cloneAnyMap(entry.InspectionMetadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["name"] = entry.DisplayName
	metadata["account_id"] = entry.AccountID
	metadata["user_id"] = entry.UserID
	metadata["plan_type"] = entry.PlanType
	if entry.UsingAPI != nil {
		metadata["using_api"] = *entry.UsingAPI
	}
	attributes := make(map[string]string, len(entry.InspectionAttrs)+3)
	for key, value := range entry.InspectionAttrs {
		attributes[key] = value
	}
	attributes["inspection_revision"] = fmt.Sprintf("%d", entry.Revision)
	if entry.RuntimeOnly {
		attributes["runtime_only"] = "true"
	}
	if entry.VirtualSource != "" {
		attributes["virtual_source"] = entry.VirtualSource
	}
	auth := &Auth{
		ID: entry.ID, Index: entry.AuthIndex, Provider: entry.Provider, Label: entry.Label,
		FileName: entry.Name, Status: AuthStatus(entry.Status), StatusMessage: entry.StatusMessage,
		Disabled: entry.Disabled, Unavailable: entry.Unavailable, UpdatedAt: entry.UpdatedAt,
		NextRefreshAfter: entry.NextRefreshAfter, Metadata: metadata, Attributes: attributes,
	}
	return auth
}

func authRevisionOf(auth *Auth) int64 {
	if auth == nil || auth.Attributes == nil {
		return 0
	}
	var revision int64
	_, _ = fmt.Sscan(auth.Attributes["inspection_revision"], &revision)
	return revision
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	raw, _ := json.Marshal(source)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func authAttribute(auth *Auth, key string) string {
	if auth == nil || auth.Attributes == nil {
		return ""
	}
	return auth.Attributes[key]
}
