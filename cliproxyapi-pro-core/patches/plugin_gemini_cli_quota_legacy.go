package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	proquota "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/quota"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	legacyGeminiCLIProvider      = "gemini-cli"
	legacyGeminiCLICodeAssistURL = "https://cloudcode-pa.googleapis.com/v1internal:"
	legacyGeminiCLIMaxBodyBytes  = 4 << 20
	legacyGeminiCLIGoogleOneAI   = "GOOGLE_ONE_AI"
)

type legacyGeminiCLIHTTPError struct {
	status int
}

func (err legacyGeminiCLIHTTPError) Error() string {
	return fmt.Sprintf("status %d", err.status)
}

func (err legacyGeminiCLIHTTPError) HTTPStatus() int {
	return err.status
}

func (h *Host) legacyQuotaProviderForRecord(record capabilityRecord) (string, bool) {
	if h == nil || record.plugin.Capabilities.QuotaProvider != nil {
		return "", false
	}
	executor := record.plugin.Capabilities.Executor
	if executor == nil || h.isPluginFused(record.id) {
		return "", false
	}
	provider, okProvider := h.executorProvider(record, executor)
	if !okProvider || provider != legacyGeminiCLIProvider {
		return "", false
	}
	return provider, true
}

func (h *Host) legacyQuotaAdapter(provider string) (capabilityRecord, bool) {
	if h == nil || normalizeProviderID(provider) != legacyGeminiCLIProvider {
		return capabilityRecord{}, false
	}
	for _, record := range h.activeRecords() {
		if candidate, okCandidate := h.legacyQuotaProviderForRecord(record); okCandidate && candidate == legacyGeminiCLIProvider {
			return record, true
		}
	}
	return capabilityRecord{}, false
}

func (h *Host) fetchLegacyGeminiCLIQuota(ctx context.Context, auth *coreauth.Auth) (pluginapi.QuotaFetchResponse, error) {
	projectID := legacyGeminiCLIProjectID(auth)
	if projectID == "" {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("gemini-cli project_id is required")
	}

	var quotaPayload map[string]any
	if errQuota := h.callLegacyGeminiCLIEndpoint(ctx, auth, "retrieveUserQuota", map[string]any{"project": projectID}, &quotaPayload); errQuota != nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("retrieve user quota: %w", errQuota)
	}
	observedAt := time.Now().UnixMilli()
	items := proquota.GeminiCLIQuotaItems(quotaPayload)
	if len(items) == 0 {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("retrieve user quota returned no supported buckets")
	}
	response := pluginapi.QuotaFetchResponse{Snapshot: pluginapi.QuotaSnapshot{
		SchemaVersion: pluginapi.QuotaSnapshotSchemaVersion,
		Provider:      legacyGeminiCLIProvider,
		ObservedAtMS:  observedAt,
		Items:         items,
		Metadata:      map[string]any{"project_id": projectID, "quota_mode": "legacy-adapter"},
	}}

	planRequest := map[string]any{
		"cloudaicompanionProject": projectID,
		"metadata": map[string]any{
			"ideType": "IDE_UNSPECIFIED", "platform": "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI", "duetProject": projectID,
		},
	}
	var planPayload map[string]any
	if errPlan := h.callLegacyGeminiCLIEndpoint(ctx, auth, "loadCodeAssist", planRequest, &planPayload); errPlan != nil {
		response.PlanUnavailable = true
		response.PlanError = errPlan.Error()
		return response, nil
	}
	response.Snapshot.Plan = proquota.GeminiCLIQuotaPlan(planPayload, observedAt)
	if response.Snapshot.Plan == nil {
		response.PlanUnavailable = true
		response.PlanError = "load code assist returned no supported tier"
	}
	return response, nil
}

func (h *Host) callLegacyGeminiCLIEndpoint(ctx context.Context, auth *coreauth.Auth, endpoint string, body any, out any) error {
	manager := h.currentAuthManager()
	if manager == nil {
		return fmt.Errorf("core auth manager is unavailable")
	}
	raw, errMarshal := json.Marshal(body)
	if errMarshal != nil {
		return fmt.Errorf("marshal request: %w", errMarshal)
	}
	req, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, legacyGeminiCLICodeAssistURL+endpoint, strings.NewReader(string(raw)))
	if errRequest != nil {
		return fmt.Errorf("build request: %w", errRequest)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, errDo := manager.HttpRequest(ctx, auth, req)
	if errDo != nil {
		return errDo
	}
	defer resp.Body.Close()
	responseBody, errRead := io.ReadAll(io.LimitReader(resp.Body, legacyGeminiCLIMaxBodyBytes+1))
	if errRead != nil {
		return fmt.Errorf("read response: %w", errRead)
	}
	if len(responseBody) > legacyGeminiCLIMaxBodyBytes {
		return fmt.Errorf("response exceeds %d bytes", legacyGeminiCLIMaxBodyBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return legacyGeminiCLIHTTPError{status: resp.StatusCode}
	}
	if errDecode := json.Unmarshal(responseBody, out); errDecode != nil {
		return fmt.Errorf("decode response: %w", errDecode)
	}
	return nil
}

func legacyGeminiCLIProjectID(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, key := range []string{"project_id", "projectId", "gemini_virtual_project"} {
		if value := proquota.GeminiCLIFirstProjectID(auth.Attributes[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"project_id", "projectId", "gemini_virtual_project"} {
		if value, _ := auth.Metadata[key].(string); proquota.GeminiCLIFirstProjectID(value) != "" {
			return proquota.GeminiCLIFirstProjectID(value)
		}
	}
	var storage map[string]any
	if json.Unmarshal(storageJSONFromAuth(auth), &storage) == nil {
		for _, key := range []string{"project_id", "projectId", "gemini_virtual_project"} {
			if value, _ := storage[key].(string); proquota.GeminiCLIFirstProjectID(value) != "" {
				return proquota.GeminiCLIFirstProjectID(value)
			}
		}
	}
	return ""
}
