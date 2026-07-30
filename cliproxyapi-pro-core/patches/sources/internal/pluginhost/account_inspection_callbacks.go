package pluginhost

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	accountInspectionDefaultResponseLimit = int64(4 * 1024 * 1024)
	accountInspectionMaximumResponseLimit = int64(8 * 1024 * 1024)
)

func requireProObservabilityAuthCallback(ctx context.Context) error {
	if hostCallbackPluginIDFromContext(ctx) != proObservabilityPluginID {
		return fmt.Errorf("account inspection auth callback is restricted to %s", proObservabilityPluginID)
	}
	return nil
}

func authRevision(auth *coreauth.Auth) int64 {
	if auth == nil || auth.UpdatedAt.IsZero() {
		return 0
	}
	return auth.UpdatedAt.UnixNano()
}

func verifyAuthRevision(auth *coreauth.Auth, expected int64) error {
	if expected > 0 && authRevision(auth) != expected {
		return fmt.Errorf("auth revision conflict")
	}
	return nil
}

func (h *Host) inspectionAuthEntry(auth *coreauth.Auth) pluginapi.HostAuthFileEntry {
	if auth == nil {
		return pluginapi.HostAuthFileEntry{}
	}
	entry := h.buildHostAuthFileEntry(auth)
	if entry == nil {
		auth.EnsureIndex()
		entry = &pluginapi.HostAuthFileEntry{
			ID: auth.ID, AuthIndex: auth.Index, Name: firstInspectionValue(auth.FileName, auth.ID),
			Type: auth.Provider, Provider: auth.Provider, Label: auth.Label,
			Status: string(auth.Status), StatusMessage: auth.StatusMessage,
			Disabled: auth.Disabled, Unavailable: auth.Unavailable, RuntimeOnly: isRuntimeOnlyAuth(auth),
			UpdatedAt: auth.UpdatedAt,
		}
	}
	entry.Revision = authRevision(auth)
	entry.NextRefreshAfter = auth.NextRefreshAfter
	entry.DisplayName = inspectionMetadataString(auth, "name", "display_name")
	if strings.TrimSpace(entry.Email) == "" {
		entry.Email = inspectionMetadataString(auth, "email")
	}
	entry.AccountID = inspectionMetadataString(auth, "account_id", "accountId")
	entry.UserID = inspectionMetadataString(auth, "user_id", "userId")
	entry.PlanType = inspectionMetadataString(auth, "plan_type", "planType")
	if value, ok := inspectionMetadataBool(auth, "using_api", "usingApi"); ok {
		entry.UsingAPI = &value
	}
	if coreauth.IsPluginVirtualAuth(auth) {
		entry.VirtualSource = strings.TrimSpace(authAttribute(auth, coreauth.AttributeVirtualSource))
		if entry.VirtualSource == "" {
			entry.VirtualSource = strings.TrimSpace(authAttribute(auth, "path"))
		}
	}
	entry.InspectionMetadata = safeInspectionMetadata(auth)
	entry.InspectionAttributes = safeInspectionAttributes(auth)
	return *entry
}

func safeInspectionMetadata(auth *coreauth.Auth) map[string]any {
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	out := make(map[string]any)
	for _, key := range []string{
		"name", "display_name", "email", "account_id", "accountId", "chatgpt_account_id", "chatgptAccountId",
		"user_id", "userId", "x_user_id", "xUserId", "subject", "sub", "id", "plan_type", "planType",
		"base_url", "auth_kind", "chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil",
		"subscription_active_until", "subscriptionActiveUntil",
	} {
		if value, ok := auth.Metadata[key]; ok {
			out[key] = value
		}
	}
	if claims := inspectionIDTokenClaims(auth.Metadata["id_token"]); claims != nil {
		for _, key := range []string{
			"chatgpt_account_id", "chatgptAccountId", "account_id", "accountId", "user_id", "userId", "sub", "id",
			"chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil", "subscription_active_until", "subscriptionActiveUntil",
		} {
			if _, exists := out[key]; !exists {
				if value, ok := claims[key]; ok {
					out[key] = value
				}
			}
		}
	}
	for _, key := range []string{"installed", "web"} {
		if nested, ok := auth.Metadata[key].(map[string]any); ok {
			safe := make(map[string]any)
			for _, nestedKey := range []string{"project_id", "projectId"} {
				if value, exists := nested[nestedKey]; exists {
					safe[nestedKey] = value
				}
			}
			if len(safe) > 0 {
				out[key] = safe
			}
		}
	}
	if subscription, ok := auth.Metadata["subscription"].(map[string]any); ok {
		safe := make(map[string]any)
		for _, key := range []string{"active_until", "activeUntil"} {
			if value, exists := subscription[key]; exists {
				safe[key] = value
			}
		}
		if len(safe) > 0 {
			out["subscription"] = safe
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func safeInspectionAttributes(auth *coreauth.Auth) map[string]string {
	if auth == nil {
		return nil
	}
	out := make(map[string]string)
	for _, key := range []string{"base_url", "using_api", "auth_kind", "runtime_only", "account_id", "accountId", "user_id", "userId", "plan_type", "planType"} {
		if value := strings.TrimSpace(authAttribute(auth, key)); value != "" {
			out[key] = value
		}
	}
	if strings.TrimSpace(authAttribute(auth, "api_key")) != "" {
		out["api_key_configured"] = "true"
	}
	if source := strings.TrimSpace(authAttribute(auth, "source")); strings.HasPrefix(strings.ToLower(source), "config:") {
		out["source"] = "config:"
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstInspectionValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func inspectionMetadataString(auth *coreauth.Auth, keys ...string) string {
	if auth == nil {
		return ""
	}
	for _, source := range []map[string]any{auth.Metadata, inspectionMetadataMap(auth.Metadata, "id_token")} {
		for _, key := range keys {
			if source != nil {
				if value, ok := source[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	if claims := inspectionIDTokenClaims(auth.Metadata["id_token"]); claims != nil {
		for _, key := range keys {
			if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func inspectionIDTokenClaims(raw any) map[string]any {
	if mapped, ok := raw.(map[string]any); ok {
		return mapped
	}
	token, _ := raw.(string)
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal([]byte(token), &claims) == nil {
		return claims
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

func inspectionMetadataMap(source map[string]any, key string) map[string]any {
	if source == nil {
		return nil
	}
	value, _ := source[key].(map[string]any)
	return value
}

func inspectionMetadataBool(auth *coreauth.Auth, keys ...string) (bool, bool) {
	if auth == nil || auth.Metadata == nil {
		return false, false
	}
	for _, key := range keys {
		switch value := auth.Metadata[key].(type) {
		case bool:
			return value, true
		case string:
			if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
				return parsed, true
			}
		}
	}
	return false, false
}

func (h *Host) callHostAuthInspectionList(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	if len(bytesTrimSpace(raw)) > 0 {
		var ignored map[string]any
		if err := json.Unmarshal(raw, &ignored); err != nil {
			return nil, fmt.Errorf("decode auth inspection list request: %w", err)
		}
	}
	manager := h.currentAuthManager()
	if manager == nil {
		return nil, fmt.Errorf("core auth manager unavailable")
	}
	auths := manager.List()
	entries := make([]pluginapi.HostAuthFileEntry, 0, len(auths))
	for _, auth := range auths {
		if auth != nil {
			entries = append(entries, h.inspectionAuthEntry(auth))
		}
	}
	return marshalRPCResult(pluginapi.HostAuthInspectionListResponse{Auths: entries})
}

func (h *Host) callHostAuthRefresh(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	var request pluginapi.HostAuthRefreshRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth refresh request: %w", err)
	}
	auth, err := h.authByIndex(request.AuthIndex)
	if err != nil {
		return nil, err
	}
	manager := h.currentAuthManager()
	var updated *coreauth.Auth
	var refreshed bool
	if request.Force {
		updated, refreshed, err = manager.ForceRefreshForInspection(ctx, auth.ID)
	} else {
		updated, refreshed, err = manager.RefreshIfDueForInspection(ctx, auth.ID)
	}
	if err != nil {
		return nil, err
	}
	if updated == nil {
		updated, err = h.authByIndex(request.AuthIndex)
		if err != nil {
			return nil, err
		}
	}
	return marshalRPCResult(pluginapi.HostAuthRefreshResponse{
		Triggered: refreshed, Refreshed: refreshed, Auth: h.inspectionAuthEntry(updated),
	})
}

func (h *Host) callHostAuthHTTPDo(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	var request pluginapi.HostAuthHTTPRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth HTTP request: %w", err)
	}
	auth, err := h.authByIndex(request.AuthIndex)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	parsedURL, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid auth HTTP URL")
	}
	timeout := request.TimeoutMS
	if timeout <= 0 {
		timeout = 15000
	}
	if timeout < 1000 {
		timeout = 1000
	}
	if timeout > 30000 {
		timeout = 30000
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	manager := h.currentAuthManager()
	httpReq, err := manager.NewHttpRequest(reqCtx, auth, method, parsedURL.String(), request.Body, request.Headers.Clone())
	if err != nil {
		return nil, err
	}
	response, err := manager.HttpRequest(reqCtx, auth, httpReq)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	limit := request.MaxResponseBytes
	if limit <= 0 {
		limit = accountInspectionDefaultResponseLimit
	}
	if limit > accountInspectionMaximumResponseLimit {
		limit = accountInspectionMaximumResponseLimit
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	truncated := int64(len(body)) > limit
	if truncated {
		body = body[:limit]
	}
	return marshalRPCResult(pluginapi.HostAuthHTTPResponse{
		StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: body, Truncated: truncated,
	})
}

func inspectionErrorSource(auth *coreauth.Auth) string {
	if auth == nil || auth.Metadata == nil {
		return ""
	}
	lastError, _ := auth.Metadata["last_error"].(map[string]any)
	source, _ := lastError["source"].(string)
	return strings.TrimSpace(source)
}

func applyInspectionHealthPatch(auth *coreauth.Auth, request pluginapi.HostAuthHealthPatchRequest) {
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	if request.Disabled != nil {
		auth.Disabled = *request.Disabled
		delete(auth.Metadata, "request_protection")
		auth.Metadata["disabled"] = auth.Disabled
		if auth.Disabled {
			auth.Status = coreauth.StatusDisabled
			auth.StatusMessage = "disabled by scheduled account inspection"
		} else if source := inspectionErrorSource(auth); source != "account_inspection" {
			if source != "" || auth.LastError != nil {
				auth.Status = coreauth.StatusError
				auth.Unavailable = true
				if auth.LastError != nil {
					auth.StatusMessage = strings.TrimSpace(auth.LastError.Message)
				}
			} else {
				auth.Status = coreauth.StatusActive
				auth.StatusMessage = ""
				auth.Unavailable = false
			}
		}
	}
	if request.ClearError && inspectionErrorSource(auth) == "account_inspection" {
		auth.LastError = nil
		delete(auth.Metadata, "last_error")
		if !auth.Disabled {
			auth.Status = coreauth.StatusActive
			auth.StatusMessage = ""
			auth.Unavailable = false
		}
	}
	if request.Error != nil {
		auth.LastError = &coreauth.Error{
			Code: request.Error.Code, Message: request.Error.Message,
			HTTPStatus: request.Error.HTTPStatus, Retryable: request.Error.Retryable,
		}
		auth.Metadata["last_error"] = map[string]any{
			"code": request.Error.Code, "message": request.Error.Message,
			"http_status": request.Error.HTTPStatus, "retryable": request.Error.Retryable,
			"source": "account_inspection", "updated_at_ms": time.Now().UnixMilli(),
		}
		auth.Status = coreauth.StatusError
		auth.StatusMessage = request.Error.Message
		auth.Unavailable = true
	}
	auth.UpdatedAt = time.Now()
}

func (h *Host) callHostAuthHealthPatch(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	var request pluginapi.HostAuthHealthPatchRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth health patch request: %w", err)
	}
	auth, err := h.authByIndex(request.AuthIndex)
	if err != nil {
		return nil, err
	}
	if err = verifyAuthRevision(auth, request.ExpectedRevision); err != nil {
		return nil, err
	}
	updated := auth.Clone()
	applyInspectionHealthPatch(updated, request)
	updated, err = h.currentAuthManager().Update(ctx, updated)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("auth disappeared during health patch")
	}
	return marshalRPCResult(pluginapi.HostAuthHealthPatchResponse{Auth: h.inspectionAuthEntry(updated)})
}

func sameInspectionSource(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func inspectionAuthSourcePath(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if coreauth.IsPluginVirtualAuth(auth) {
		if path := strings.TrimSpace(authAttribute(auth, coreauth.AttributeVirtualSource)); path != "" {
			return path
		}
	}
	return strings.TrimSpace(authAttribute(auth, "path"))
}

func (h *Host) callHostAuthDelete(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	var request pluginapi.HostAuthDeleteRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth delete request: %w", err)
	}
	auth, err := h.authByIndex(request.AuthIndex)
	if err != nil {
		return nil, err
	}
	if err = verifyAuthRevision(auth, request.ExpectedRevision); err != nil {
		return nil, err
	}
	manager := h.currentAuthManager()
	sourcePath := inspectionAuthSourcePath(auth)
	if sourcePath == "" {
		return nil, fmt.Errorf("auth source path unavailable")
	}
	if coreauth.IsPluginVirtualAuth(auth) {
		count := 0
		for _, candidate := range manager.List() {
			if candidate != nil && coreauth.IsPluginVirtualAuth(candidate) && sameInspectionSource(inspectionAuthSourcePath(candidate), sourcePath) {
				count++
			}
		}
		if count > 1 {
			return nil, fmt.Errorf("cannot delete one plugin virtual auth from a shared source file")
		}
	}
	if !filepath.IsAbs(sourcePath) {
		if absolute, errAbs := filepath.Abs(sourcePath); errAbs == nil {
			sourcePath = absolute
		}
	}
	if err = os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("delete auth source: %w", err)
	}
	store := sdkauth.GetTokenStore()
	if setter, ok := store.(interface{ SetBaseDir(string) }); ok {
		setter.SetBaseDir(h.resolvedAuthDir())
	}
	if store != nil {
		_ = store.Delete(ctx, sourcePath)
	}
	for _, candidate := range manager.List() {
		if candidate != nil && (candidate.ID == auth.ID || sameInspectionSource(inspectionAuthSourcePath(candidate), sourcePath)) {
			manager.Remove(ctx, candidate.ID)
		}
	}
	_, _, _ = h.DeleteAuthRuntimeState(ctx, pluginapi.AuthRuntimeStateDeleteRequest{
		AuthID: auth.ID, AuthIndex: auth.Index, FileName: firstInspectionValue(auth.FileName, filepath.Base(sourcePath)),
	})
	return marshalRPCResult(pluginapi.HostAuthDeleteResponse{Name: firstInspectionValue(auth.FileName, filepath.Base(sourcePath))})
}

func (h *Host) callHostAuthQuotaFetch(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProObservabilityAuthCallback(ctx); err != nil {
		return nil, err
	}
	var request pluginapi.HostAuthQuotaFetchRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth quota fetch request: %w", err)
	}
	auth, err := h.authByIndex(request.AuthIndex)
	if err != nil {
		return nil, err
	}
	result := h.FetchQuota(ctx, auth, nil)
	response := pluginapi.HostAuthQuotaFetchResponse{
		Handled: result.Handled, Snapshot: result.Snapshot, UpstreamStatus: result.UpstreamStatus,
	}
	if result.Err != nil {
		response.Error = result.Err.Error()
	}
	if result.Auth != nil {
		if _, updateErr := h.currentAuthManager().Update(ctx, result.Auth); updateErr != nil {
			return nil, fmt.Errorf("apply quota auth update: %w", updateErr)
		}
	}
	return marshalRPCResult(response)
}
