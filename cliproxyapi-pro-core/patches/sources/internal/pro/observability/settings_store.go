package observability

import (
	"context"
	"encoding/json"
	"strings"

	probackup "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/backup"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

// SettingsStore adapts the observability-owned shared SQLite repository to the
// module-facing settings port. The dependency points from the infrastructure
// adapter to the port, so business modules never import observability or the
// historical embeddedusage façade.
type SettingsStore struct{}

func NewSettingsStore() SettingsStore { return SettingsStore{} }

type uncoordinatedSettingsStore struct{ SettingsStore }

func (uncoordinatedSettingsStore) Put(ctx context.Context, item settings.Item) error {
	return setProSetting(ctx, ProSetting{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	})
}

func (uncoordinatedSettingsStore) Delete(ctx context.Context, namespace string) error {
	return deleteProSetting(ctx, namespace)
}

func (SettingsStore) ExecuteWrite(
	ctx context.Context,
	operation func(context.Context, settings.Store) error,
) error {
	if operation == nil {
		return nil
	}
	return probackup.Default.ExecuteWrite(ctx, func(ctx context.Context) error {
		return operation(ctx, uncoordinatedSettingsStore{})
	})
}

func (SettingsStore) Get(ctx context.Context, namespace string) (settings.Item, bool, error) {
	stored, found, err := GetProSetting(ctx, namespace)
	if err != nil || !found {
		return settings.Item{}, found, err
	}
	return settingItem(stored), true, nil
}

func (SettingsStore) Put(ctx context.Context, item settings.Item) error {
	return SetProSetting(ctx, ProSetting{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	})
}

func (SettingsStore) Delete(ctx context.Context, namespace string) error {
	return DeleteProSetting(ctx, namespace)
}

func (SettingsStore) GetPlanSnapshot(ctx context.Context, provider, fileName, authIndex string) (settings.PlanSnapshot, bool, error) {
	provider = strings.TrimSpace(provider)
	fileName = strings.TrimSpace(fileName)
	authIndex = strings.TrimSpace(authIndex)
	if fileName == "" {
		fileName = authIndex
	}
	entries, err := GetQuotaCache(ctx, provider, fileName)
	if err != nil {
		return settings.PlanSnapshot{}, false, err
	}
	var selected *QuotaCacheEntry
	for _, entry := range entries {
		entryAuthIndex := strings.TrimSpace(entry.AuthIndex)
		if (authIndex == "" && entryAuthIndex != "") || (authIndex != "" && entryAuthIndex != "" && entryAuthIndex != authIndex) {
			continue
		}
		if !isAuthCardQuotaSnapshotCompatible(provider, entry.Data) {
			continue
		}
		if selected == nil || preferredQuotaCacheEntry(provider, entry, *selected) {
			candidate := entry
			selected = &candidate
		}
	}
	if selected == nil {
		return settings.PlanSnapshot{}, false, nil
	}
	return settings.PlanSnapshot{
		Data: append([]byte(nil), selected.Data...), ObservedAtMS: selected.ObservedAt,
	}, true, nil
}

// isAuthCardQuotaSnapshotCompatible mirrors the Management auth-card
// persistence contract. Only rows that the card can hydrate participate in
// account-policy selection; provider-neutral plugin rows for other providers
// remain persisted without displacing a valid inspection snapshot.
func isAuthCardQuotaSnapshotCompatible(provider string, raw []byte) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "gemini-cli" && isNormalizedGeminiQuotaSnapshot(raw) {
		return true
	}
	payload := map[string]any{}
	if json.Unmarshal(raw, &payload) != nil || payload == nil {
		return false
	}
	status, ok := payload["status"].(string)
	if !ok {
		return false
	}
	switch status {
	case "idle", "loading", "error":
		return true
	case "success":
	default:
		return false
	}
	switch provider {
	case "antigravity":
		groups, okGroups := payload["groups"].([]any)
		if !okGroups {
			return false
		}
		for _, rawGroup := range groups {
			group, okGroup := rawGroup.(map[string]any)
			if !okGroup {
				return false
			}
			if _, okBuckets := group["buckets"].([]any); !okBuckets {
				return false
			}
		}
		return true
	case "claude", "codex":
		_, ok = payload["windows"].([]any)
		return ok
	case "gemini-cli":
		_, ok = payload["buckets"].([]any)
		return ok
	case "kimi":
		_, ok = payload["rows"].([]any)
		return ok
	case "xai":
		_, ok = payload["billing"].(map[string]any)
		return ok
	default:
		return false
	}
}

// preferredQuotaCacheEntry mirrors Management's auth-card cache selection.
// Both consumers must resolve duplicate inspection/plugin rows to the same
// effective snapshot before applying provider-specific plan semantics.
func preferredQuotaCacheEntry(provider string, candidate, current QuotaCacheEntry) bool {
	if strings.EqualFold(strings.TrimSpace(provider), "gemini-cli") {
		candidateNormalized := isNormalizedGeminiQuotaSnapshot(candidate.Data)
		currentNormalized := isNormalizedGeminiQuotaSnapshot(current.Data)
		if candidateNormalized != currentNormalized {
			return candidateNormalized
		}
	}
	candidateFreshness := [...]int64{candidate.ObservedAt, candidate.CachedAt, candidate.StoredAt, candidate.Revision}
	currentFreshness := [...]int64{current.ObservedAt, current.CachedAt, current.StoredAt, current.Revision}
	for index := range candidateFreshness {
		if candidateFreshness[index] == currentFreshness[index] {
			continue
		}
		return candidateFreshness[index] > currentFreshness[index]
	}
	return false
}

func isNormalizedGeminiQuotaSnapshot(raw []byte) bool {
	payload := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &payload) != nil {
		return false
	}
	if _, hasStatus := payload["status"]; hasStatus {
		return false
	}
	var items []json.RawMessage
	return json.Unmarshal(payload["items"], &items) == nil && items != nil
}

func (SettingsStore) Subscribe(namespace string, apply func(context.Context, settings.Item) error) func() {
	if apply == nil {
		return func() {}
	}
	return RegisterProSettingConsumer(namespace, func(ctx context.Context, item ProSetting) error {
		return apply(ctx, settingItem(item))
	})
}

func settingItem(item ProSetting) settings.Item {
	return settings.Item{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	}
}
