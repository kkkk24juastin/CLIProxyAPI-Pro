package management

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	proinspection "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/inspection"
	proquota "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/quota"
)

func TestAccountInspectionDeepProbesUnknownXAIQuota(t *testing.T) {
	decision := accountInspectionDecision{Action: accountInspectionActionKeep}
	if !proinspection.ShouldDeepProbe(decision) {
		t.Fatal("unknown xAI quota should allow an explicitly enabled deep probe")
	}
}

func TestAntigravityQuotaURLsUseSummaryEndpoint(t *testing.T) {
	for _, url := range antigravityQuotaURLs() {
		if !strings.Contains(url, "retrieveUserQuotaSummary") {
			t.Fatalf("antigravity quota url = %q, want retrieveUserQuotaSummary", url)
		}
	}
}

func TestXAIRequestHeadersIncludeGrokClientAndUserID(t *testing.T) {
	auth := &coreauth.Auth{
		Provider: "xai",
		Metadata: map[string]any{"sub": "user-123"},
	}
	headers := xaiRequestHeaders(auth)
	if headers["x-xai-token-auth"] != "xai-grok-cli" {
		t.Fatalf("x-xai-token-auth = %q", headers["x-xai-token-auth"])
	}
	if headers["x-grok-client-version"] != "0.2.91" {
		t.Fatalf("x-grok-client-version = %q", headers["x-grok-client-version"])
	}
	if headers["x-userid"] != "user-123" {
		t.Fatalf("x-userid = %q, want user-123", headers["x-userid"])
	}
}

func TestXAIInspectionUsesExecutorHTTPRequest(t *testing.T) {
	if !accountInspectionShouldUseExecutorHTTPRequest(&coreauth.Auth{Provider: "xai"}) {
		t.Fatal("accountInspectionShouldUseExecutorHTTPRequest(xai) = false, want true")
	}
}

func TestXAIInspectionRoutesByUsingAPI(t *testing.T) {
	tests := []struct {
		name          string
		usingAPI      string
		wantURLs      []string
		forbiddenPath string
	}{
		{
			name:          "official api",
			usingAPI:      "true",
			wantURLs:      []string{"https://api.x.ai/v1/chat/completions"},
			forbiddenPath: "/billing",
		},
		{
			name:     "grok cli",
			usingAPI: "false",
			wantURLs: []string{
				"https://cli-chat-proxy.grok.com/v1/billing?format=credits",
				"https://cli-chat-proxy.grok.com/v1/billing",
			},
			forbiddenPath: "/chat/completions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &xaiInspectionRoutingExecutor{}
			manager := coreauth.NewManager(nil, nil, nil)
			manager.RegisterExecutor(executor)
			scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
			auth := &coreauth.Auth{
				Provider: "xai",
				Attributes: map[string]string{
					"api_key":   "test-token",
					"base_url":  "https://api.x.ai/v1",
					"using_api": tt.usingAPI,
				},
			}
			decision, status, err := scheduler.inspectXAI(context.Background(), accountInspectionAccount{
				Auth:      auth,
				Provider:  "xai",
				FileName:  tt.name + ".json",
				AuthIndex: tt.name,
			}, accountInspectionSettings{Timeout: 3_000, XAIDeepProbeModel: "grok-4.5"})
			if err != nil || status == nil || *status != http.StatusOK || decision.Action != accountInspectionActionKeep {
				t.Fatalf("inspectXAI() = decision:%#v status:%v err:%v", decision, status, err)
			}
			gotURLs := make([]string, 0, len(executor.requests))
			for _, request := range executor.requests {
				gotURLs = append(gotURLs, request.URL.String())
				if strings.Contains(request.URL.String(), tt.forbiddenPath) {
					t.Fatalf("inspectXAI() requested forbidden URL %q", request.URL.String())
				}
			}
			if strings.Join(gotURLs, "\n") != strings.Join(tt.wantURLs, "\n") {
				t.Fatalf("inspectXAI() URLs = %#v, want %#v", gotURLs, tt.wantURLs)
			}
		})
	}
}

func TestXAIBillingURLMatchesUpstreamQuotaConfig(t *testing.T) {
	if got := xaiBillingURL(); got != "https://cli-chat-proxy.grok.com/v1/billing" {
		t.Fatalf("xaiBillingURL() = %q, want upstream billing endpoint", got)
	}
	if got := xaiBillingWeeklyURL(); got != "https://cli-chat-proxy.grok.com/v1/billing?format=credits" {
		t.Fatalf("xaiBillingWeeklyURL() = %q, want upstream weekly billing endpoint", got)
	}
}

func TestXAIDeepProbeDefaultsAndNormalization(t *testing.T) {
	defaults := proinspection.DefaultSettings()
	if defaults.XAIDeepProbeEnabled {
		t.Fatal("xAI deep probe should be disabled by default")
	}
	if defaults.XAIDeepProbeModel != "grok-4.5" {
		t.Fatalf("default xAI deep probe model = %q, want grok-4.5", defaults.XAIDeepProbeModel)
	}

	normalized := normalizeAccountInspectionSchedule(accountInspectionSchedule{Settings: accountInspectionSettings{
		XAIDeepProbeEnabled: true,
		XAIDeepProbeModel:   "   ",
	}})
	if !normalized.Settings.XAIDeepProbeEnabled || normalized.Settings.XAIDeepProbeModel != "grok-4.5" {
		t.Fatalf("normalized xAI deep probe settings = enabled:%v model:%q", normalized.Settings.XAIDeepProbeEnabled, normalized.Settings.XAIDeepProbeModel)
	}
}

func TestXAIResponsesURLUsesConfiguredBaseURL(t *testing.T) {
	if got := xaiResponsesURL(nil); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(nil) = %q", got)
	}
	oauth := &coreauth.Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1", "auth_kind": "oauth"}}
	if got := xaiResponsesURL(oauth); got != "https://cli-chat-proxy.grok.com/v1/responses" {
		t.Fatalf("xaiResponsesURL(oauth) = %q", got)
	}
	api := &coreauth.Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1", "using_api": "true"}}
	if got := xaiResponsesURL(api); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(api) = %q", got)
	}
	if got := xaiOfficialChatURL(api); got != "https://api.x.ai/v1/chat/completions" {
		t.Fatalf("xaiOfficialChatURL(api) = %q", got)
	}
	metadataOAuth := &coreauth.Auth{Metadata: map[string]any{"base_url": "https://api.x.ai/v1", "using_api": false}}
	if got := xaiResponsesURL(metadataOAuth); got != "https://cli-chat-proxy.grok.com/v1/responses" {
		t.Fatalf("xaiResponsesURL(metadataOAuth) = %q", got)
	}
	defaultAPI := &coreauth.Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1"}}
	if got := xaiResponsesURL(defaultAPI); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(defaultAPI) = %q", got)
	}
	auth := &coreauth.Auth{Attributes: map[string]string{"base_url": "https://xai.example/v1/"}}
	if got := xaiResponsesURL(auth); got != "https://xai.example/v1/responses" {
		t.Fatalf("xaiResponsesURL(custom) = %q", got)
	}
	headers := xaiDeepProbeHeaders(oauth)
	if headers["x-xai-token-auth"] != "xai-grok-cli" || headers["Accept"] != "text/event-stream" {
		t.Fatalf("xaiDeepProbeHeaders(oauth) = %#v", headers)
	}
	apiHeaders := xaiDeepProbeHeaders(api)
	if apiHeaders["x-xai-token-auth"] != "" || apiHeaders["Authorization"] != "Bearer $TOKEN$" {
		t.Fatalf("xaiDeepProbeHeaders(api) = %#v", apiHeaders)
	}
	officialHeaders := xaiOfficialAPIHeaders()
	if officialHeaders["x-xai-token-auth"] != "" || officialHeaders["Accept"] != "application/json" {
		t.Fatalf("xaiOfficialAPIHeaders() = %#v", officialHeaders)
	}
	customOAuth := &coreauth.Auth{Attributes: map[string]string{"base_url": "https://xai.example/v1", "using_api": "false"}}
	customHeaders := xaiDeepProbeHeaders(customOAuth)
	if customHeaders["x-xai-token-auth"] != "" {
		t.Fatalf("xaiDeepProbeHeaders(customOAuth) = %#v", customHeaders)
	}
}

func TestBuildXAIOfficialHealthRequestAndSummary(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(proinspection.BuildXAIOfficialHealthBody(" grok-4.5 ")), &payload); err != nil {
		t.Fatalf("proinspection.BuildXAIOfficialHealthBody() JSON error = %v", err)
	}
	messages, _ := payload["messages"].([]any)
	if payload["model"] != "grok-4.5" || len(messages) != 1 || payload["stream"] != false || payload["max_tokens"] != float64(1) {
		t.Fatalf("official health payload = %#v", payload)
	}
	summary := proquota.XAIPaidHealthSummary()
	if summary["mode"] != "paid-health" || summary["planType"] != "paid" || summary["healthStatus"] != "chat-ok" {
		t.Fatalf("paid health summary = %#v", summary)
	}
	if _, exists := summary["freeQuota"]; exists {
		t.Fatalf("paid health summary contains free quota: %#v", summary)
	}
}

func TestXAIOfficialAPIQuotaDecision(t *testing.T) {
	active := xaiOfficialAPIQuotaDecision(accountInspectionAccount{}, `{"error":"credits exhausted"}`)
	if active.Action != accountInspectionActionDisable || !active.IsQuota || !strings.Contains(active.ErrorDetail, "credits exhausted") {
		t.Fatalf("active official quota decision = %#v", active)
	}
	disabled := xaiOfficialAPIQuotaDecision(accountInspectionAccount{Disabled: true}, `{"error":"credits exhausted"}`)
	if disabled.Action != accountInspectionActionKeep || !disabled.IsQuota {
		t.Fatalf("disabled official quota decision = %#v", disabled)
	}
}

func TestBuildXAIDeepProbeBodyUsesMinimalResponsesRequest(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(proinspection.BuildXAIDeepProbeBody(" grok-4.3 ")), &payload); err != nil {
		t.Fatalf("proinspection.BuildXAIDeepProbeBody() JSON error = %v", err)
	}
	input, _ := payload["input"].([]any)
	if payload["model"] != "grok-4.3" || len(input) != 1 || payload["stream"] != true || payload["store"] != false || payload["max_output_tokens"] != float64(1) {
		t.Fatalf("deep probe payload = %#v", payload)
	}
}

func TestClassifyXAIDeepProbeResponse(t *testing.T) {
	tests := []struct {
		name string
		resp accountInspectionHTTPResult
		want accountInspectionDeepProbeStatus
	}{
		{
			name: "completed sse",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"},
			want: accountInspectionDeepProbeSuccess,
		},
		{
			name: "output capped after successful execution",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: `data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n"},
			want: accountInspectionDeepProbeSuccess,
		},
		{
			name: "free usage exhausted",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusTooManyRequests, Body: `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for now."}`},
			want: accountInspectionDeepProbeQuota,
		},
		{
			name: "credits exhausted returned as forbidden",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusForbidden, Body: `{"error":{"message":"You have run out of credits or need a Grok subscription. Add credits or upgrade to SuperGrok."}}`},
			want: accountInspectionDeepProbeQuota,
		},
		{
			name: "unauthorized",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusUnauthorized, Body: `{"error":{"message":"invalid token"}}`},
			want: accountInspectionDeepProbeAuthError,
		},
		{
			name: "content filter incomplete response",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}` + "\n\n"},
			want: accountInspectionDeepProbeTransientError,
		},
		{
			name: "missing terminal response",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: "data: {\"type\":\"response.created\"}\n\n"},
			want: accountInspectionDeepProbeTransientError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyXAIDeepProbeResponse(tt.resp)
			if got != tt.want {
				t.Fatalf("classifyXAIDeepProbeResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyAntigravityDeepProbePrefersQuotaEvidenceOverAuthStatus(t *testing.T) {
	resp := accountInspectionHTTPResult{
		StatusCode: http.StatusForbidden,
		Body:       `{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}}`,
	}
	status, _ := classifyAntigravityDeepProbeResponse(resp)
	if status != accountInspectionDeepProbeQuota {
		t.Fatalf("classifyAntigravityDeepProbeResponse() = %q, want %q", status, accountInspectionDeepProbeQuota)
	}
}

func TestAntigravityDeepProbeFailoverStopsOnDeterministicClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if !shouldStopAntigravityDeepProbeFailover(status) {
			t.Fatalf("shouldStopAntigravityDeepProbeFailover(%d) = false, want true", status)
		}
	}
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		if shouldStopAntigravityDeepProbeFailover(status) {
			t.Fatalf("shouldStopAntigravityDeepProbeFailover(%d) = true, want false", status)
		}
	}
}

func TestCodexDecisionPrefersQuotaEvidenceOverUnauthorizedStatus(t *testing.T) {
	decision := codexDecision(accountInspectionAccount{}, http.StatusUnauthorized, nil, true, 95)
	if !decision.IsQuota || decision.Action != accountInspectionActionDisable {
		t.Fatalf("codexDecision() = %#v, want quota disable decision", decision)
	}
	if got := proinspection.DecisionErrorCode("codex", decision, testStatusCode(http.StatusUnauthorized)); got != "" {
		t.Fatalf("quota decision error code = %q, want empty", got)
	}
}

func TestRunXAIDeepProbeWithRetryRecoversFromEmptyResponse(t *testing.T) {
	attempts := 0
	resp, status, message, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		if attempts == 1 {
			return accountInspectionHTTPResult{StatusCode: http.StatusOK}, nil
		}
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeSuccess || message != "" {
		t.Fatalf("runXAIDeepProbeWithRetry() = resp:%+v status:%q message:%q err:%v, want success", resp, status, message, err)
	}
	if attempts != 2 {
		t.Fatalf("runXAIDeepProbeWithRetry() attempts = %d, want 2", attempts)
	}
}

func TestRunXAIDeepProbeWithRetryRecoversFromTransportError(t *testing.T) {
	attempts := 0
	_, status, _, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		if attempts == 1 {
			return accountInspectionHTTPResult{}, errors.New("temporary transport failure")
		}
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeSuccess || attempts != 2 {
		t.Fatalf("runXAIDeepProbeWithRetry() = status:%q attempts:%d err:%v, want success after 2 attempts", status, attempts, err)
	}
}

func TestRunXAIDeepProbeWithRetryDoesNotRetryContentFilter(t *testing.T) {
	attempts := 0
	_, status, message, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeTransientError || !strings.Contains(message, "content_filter") {
		t.Fatalf("runXAIDeepProbeWithRetry() = status:%q message:%q err:%v, want content_filter transient error", status, message, err)
	}
	if attempts != 1 {
		t.Fatalf("runXAIDeepProbeWithRetry() attempts = %d, want 1", attempts)
	}
}

func TestAcquireXAIDeepProbeSerializesAndHonorsCancellation(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	releaseFirst, err := scheduler.acquireXAIDeepProbe(context.Background())
	if err != nil {
		t.Fatalf("first acquireXAIDeepProbe() error = %v", err)
	}

	secondAcquired := make(chan func(), 1)
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		release, acquireErr := scheduler.acquireXAIDeepProbe(context.Background())
		if acquireErr == nil {
			secondAcquired <- release
		}
	}()
	<-secondStarted
	select {
	case release := <-secondAcquired:
		release()
		releaseFirst()
		t.Fatal("second xAI deep probe acquired before the first probe released")
	case <-time.After(50 * time.Millisecond):
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	canceledResult := make(chan error, 1)
	go func() {
		_, acquireErr := scheduler.acquireXAIDeepProbe(canceledCtx)
		canceledResult <- acquireErr
	}()
	cancel()
	select {
	case acquireErr := <-canceledResult:
		if !errors.Is(acquireErr, context.Canceled) {
			releaseFirst()
			t.Fatalf("canceled acquireXAIDeepProbe() error = %v, want context.Canceled", acquireErr)
		}
	case <-time.After(time.Second):
		releaseFirst()
		t.Fatal("canceled acquireXAIDeepProbe() did not return")
	}

	releaseFirst()
	select {
	case release := <-secondAcquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("second xAI deep probe did not acquire after release")
	}
}

func TestSummarizeInspectionHTTPBodyExtractsCompleteNestedMessage(t *testing.T) {
	want := strings.TrimSpace(strings.Repeat("capacity unavailable ", 20))
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    http.StatusServiceUnavailable,
			"message": want,
			"status":  "UNAVAILABLE",
		},
	})
	if err != nil {
		t.Fatalf("marshal error payload: %v", err)
	}
	if got := proinspection.SummarizeHTTPBody(string(body)); got != want {
		t.Fatalf("proinspection.SummarizeHTTPBody() = %q, want complete nested message %q", got, want)
	}
	if got := proinspection.HTTPErrorDetail("  " + string(body) + "\n"); got != string(body) {
		t.Fatalf("proinspection.HTTPErrorDetail() = %q, want complete body %q", got, string(body))
	}
}

func TestWithInspectionHTTPErrorDetailPreservesCompleteResponse(t *testing.T) {
	body := `{"error":{"code":"invalid_token","message":"credential rejected"},"request_id":"req-123"}`
	decision := proinspection.WithHTTPErrorDetail(
		authErrorDecision(accountInspectionAccount{}, http.StatusUnauthorized),
		"  "+body+"\n",
	)
	if decision.ErrorDetail != body {
		t.Fatalf("ErrorDetail = %q, want complete response %q", decision.ErrorDetail, body)
	}
	if decision.Action != accountInspectionActionDisable {
		t.Fatalf("Action = %q, want %q", decision.Action, accountInspectionActionDisable)
	}
}

func TestTransientDeepProbeErrorCodeTakesPriorityOverHTTPStatus(t *testing.T) {
	decision := accountInspectionDecision{DeepProbeStatus: accountInspectionDeepProbeTransientError}
	status := testStatusCode(http.StatusBadRequest)
	tests := []struct {
		provider string
		want     string
	}{
		{provider: "antigravity", want: "antigravity_deep_probe_error"},
		{provider: "xai", want: "xai_deep_probe_error"},
	}
	for _, tt := range tests {
		if got := proinspection.DecisionErrorCode(tt.provider, decision, status); got != tt.want {
			t.Fatalf("%s deep probe error code = %q, want %q", tt.provider, got, tt.want)
		}
		if !isInspectionAuthErrorCode(tt.want) {
			t.Fatalf("%s deep probe error code should be clearable after recovery", tt.provider)
		}
	}
}
