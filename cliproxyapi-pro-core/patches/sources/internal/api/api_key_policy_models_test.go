package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/home"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	proapp "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/app"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func newAPIKeyPolicyModelsServer(t *testing.T) (*Server, *apikeypolicy.Service) {
	t.Helper()
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	registryRef := registry.GetGlobalRegistry()
	registryRef.RegisterClient("api-key-models-codex", "codex", []*registry.ModelInfo{
		{ID: "shared-policy-model", Object: "model", OwnedBy: "catalog-owner", DisplayName: "Shared policy model", ContextLength: 128000},
		{ID: "codex-only-policy-model", Object: "model", OwnedBy: "catalog-owner"},
	})
	registryRef.RegisterClient("api-key-models-gemini", "gemini", []*registry.ModelInfo{
		{ID: "shared-policy-model", Object: "model", OwnedBy: "catalog-owner", Name: "shared-policy-model", DisplayName: "Shared policy model", ContextLength: 128000},
	})
	t.Cleanup(func() {
		registryRef.UnregisterClient("api-key-models-codex")
		registryRef.UnregisterClient("api-key-models-gemini")
	})

	application, err := proapp.New(context.Background(), filepath.Join(t.TempDir(), "config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	service := application.APIKeyPolicy()
	if err = service.SetTakeover(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	identity, err := apikeypolicy.NewAuthenticatedAPIKeyIdentity("test-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(context.Background(), identity, "Test key", apikeypolicy.ProfileInput{
		Name: "restricted", Providers: []string{"gemini"}, Models: []string{"shared-policy-model"},
		Mappings: []apikeypolicy.ModelMapping{{Source: "smart-policy-model", Target: "shared-policy-model"}},
	}); err != nil {
		t.Fatal(err)
	}
	return newTestServerWithOptions(t, WithProApp(application)), service
}

func requestAPIKeyPolicyModels(t *testing.T, server *Server, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer test-key")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)
	return recorder
}

func modelIDsFromPolicyResponse(t *testing.T, payload []byte, field string) []string {
	t.Helper()
	var response map[string]any
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode model response: %v; body=%s", err, payload)
	}
	items, _ := response[field].([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		model, _ := item.(map[string]any)
		id, _ := model["id"].(string)
		if id == "" {
			id, _ = model["name"].(string)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestAPIKeyPolicyModelDiscoveryFiltersOpenAIClaudeAndGemini(t *testing.T) {
	server, _ := newAPIKeyPolicyModelsServer(t)

	openAI := requestAPIKeyPolicyModels(t, server, "/v1/models", nil)
	if openAI.Code != http.StatusOK {
		t.Fatalf("OpenAI status=%d body=%s", openAI.Code, openAI.Body.String())
	}
	if got := modelIDsFromPolicyResponse(t, openAI.Body.Bytes(), "data"); !equalStringSet(got, []string{"shared-policy-model", "smart-policy-model"}) {
		t.Fatalf("OpenAI models=%v body=%s", got, openAI.Body.String())
	}
	if strings.Contains(openAI.Body.String(), "codex-only-policy-model") {
		t.Fatalf("OpenAI leaked forbidden model: %s", openAI.Body.String())
	}

	claude := requestAPIKeyPolicyModels(t, server, "/v1/models", map[string]string{"Anthropic-Version": "2023-06-01"})
	if claude.Code != http.StatusOK {
		t.Fatalf("Claude status=%d body=%s", claude.Code, claude.Body.String())
	}
	claudeIDs := modelIDsFromPolicyResponse(t, claude.Body.Bytes(), "data")
	if len(claudeIDs) != 2 {
		t.Fatalf("Claude models=%v body=%s", claudeIDs, claude.Body.String())
	}
	for _, id := range claudeIDs {
		if !strings.HasPrefix(id, "claude-fable-5-dd-") {
			t.Fatalf("Claude model %q did not preserve native cloaking", id)
		}
	}

	gemini := requestAPIKeyPolicyModels(t, server, "/v1beta/models", nil)
	if gemini.Code != http.StatusOK {
		t.Fatalf("Gemini status=%d body=%s", gemini.Code, gemini.Body.String())
	}
	if got := modelIDsFromPolicyResponse(t, gemini.Body.Bytes(), "models"); !equalStringSet(got, []string{"models/shared-policy-model", "models/smart-policy-model"}) {
		t.Fatalf("Gemini models=%v body=%s", got, gemini.Body.String())
	}

	alias := requestAPIKeyPolicyModels(t, server, "/v1beta/models/smart-policy-model", nil)
	if alias.Code != http.StatusOK || !strings.Contains(alias.Body.String(), `"name":"models/smart-policy-model"`) || !strings.Contains(alias.Body.String(), "Shared policy model") {
		t.Fatalf("Gemini alias status=%d body=%s", alias.Code, alias.Body.String())
	}
	forbidden := requestAPIKeyPolicyModels(t, server, "/v1beta/models/codex-only-policy-model", nil)
	if forbidden.Code != http.StatusNotFound || !strings.Contains(forbidden.Body.String(), `"type":"not_found"`) {
		t.Fatalf("Gemini forbidden status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
}

func TestAPIKeyPolicyModelDiscoveryCoversCodexAndGrokNativeFormats(t *testing.T) {
	server, _ := newAPIKeyPolicyModelsServer(t)

	codex := requestAPIKeyPolicyModels(t, server, "/v1/models?client_version=0.1", nil)
	if codex.Code != http.StatusOK || !strings.Contains(codex.Body.String(), "smart-policy-model") || !strings.Contains(codex.Body.String(), "shared-policy-model") || strings.Contains(codex.Body.String(), "codex-only-policy-model") {
		t.Fatalf("Codex status=%d body=%s", codex.Code, codex.Body.String())
	}

	grok := requestAPIKeyPolicyModels(t, server, "/v1/models", map[string]string{"User-Agent": "grok-shell/0.2.119"})
	if grok.Code != http.StatusOK {
		t.Fatalf("Grok status=%d body=%s", grok.Code, grok.Body.String())
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(grok.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(response.Data))
	for _, model := range response.Data {
		ids = append(ids, model.ID)
	}
	if !equalStringSet(ids, []string{"shared-policy-model", "smart-policy-model"}) {
		t.Fatalf("Grok models=%v body=%s", ids, grok.Body.String())
	}
}

func TestAPIKeyPolicyDiscoveryAndExecutionShareExactAliasPrefixAutoAndThinkingOrder(t *testing.T) {
	const target = "shared-policy-model"
	decision := apikeypolicy.RequestPolicyDecision{Mode: apikeypolicy.ModeProfile, Snapshot: &apikeypolicy.RequestPolicySnapshot{
		PolicyID: "policy", ProfileID: "profile", ProfileName: "restricted", Version: 1,
		AllowedModels: map[string]struct{}{target: {}}, AllowedProviders: map[string]struct{}{"gemini": {}},
		ModelMappings: map[string]string{
			"auto":             target,
			"tenant/public":    target,
			"custom(high)":     target,
			"models/gemini-ui": target,
		},
	}}
	visible, err := decision.FilterVisibleModels([]apikeypolicy.ModelCandidate{{ID: target, Providers: []string{"codex", "gemini"}}})
	if err != nil {
		t.Fatal(err)
	}
	visibleIDs := make([]string, 0, len(visible))
	for _, model := range visible {
		visibleIDs = append(visibleIDs, model.ID)
	}
	wantVisible := []string{target, "auto", "custom(high)", "models/gemini-ui", "tenant/public"}
	if !equalStringSet(visibleIDs, wantVisible) {
		t.Fatalf("visible aliases=%v, want %v", visibleIDs, wantVisible)
	}
	for requested, want := range map[string]string{
		"auto":               target,
		"auto(high)":         target + "(high)",
		"tenant/public":      target,
		"tenant/public(max)": target + "(max)",
		"custom(high)":       target,
		"models/gemini-ui":   target,
	} {
		if got, applyErr := decision.ApplyModel(requested); applyErr != nil || got != want {
			t.Fatalf("ApplyModel(%q)=%q, %v; want %q", requested, got, applyErr, want)
		}
	}
}

func TestAPIKeyPolicyHomeDiscoveryFiltersEveryNativeFormat(t *testing.T) {
	server, service := newAPIKeyPolicyModelsServer(t)
	payload := []byte(`{"home":[{"id":"shared-policy-model","display_name":"Shared Home Model","context_length":128000},{"id":"forbidden-home-model","display_name":"Forbidden Home Model"}]}`)
	client, commands := newAPIKeyPolicyHomeModelsClient(t, payload)
	previousHome := home.Current()
	home.SetCurrent(client)
	t.Cleanup(func() { home.SetCurrent(previousHome) })
	policies, err := service.List(context.Background())
	if err != nil || len(policies) != 1 {
		t.Fatalf("policies=%#v error=%v", policies, err)
	}
	policy := policies[0]
	if _, err = service.ReplaceProfile(context.Background(), policy.ID, policy.ActiveProfileID, policy.Version, apikeypolicy.ProfileInput{
		Name: "home", Providers: []string{"home"}, Models: []string{"shared-policy-model"},
		Mappings: []apikeypolicy.ModelMapping{{Source: "smart-policy-model", Target: "shared-policy-model"}},
	}); err != nil {
		t.Fatal(err)
	}
	identity, err := apikeypolicy.NewAuthenticatedAPIKeyIdentity("test-key")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := service.Decide(identity)
	if err != nil {
		t.Fatal(err)
	}

	server.cfg.Home = config.HomeConfig{Enabled: true}
	invoke := func(path string, headers map[string]string, action string, handler func(*gin.Context)) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request = request.WithContext(apikeypolicy.WithDecision(request.Context(), decision))
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		ginContext.Request = request
		if action != "" {
			ginContext.Params = gin.Params{{Key: "action", Value: action}}
		}
		handler(ginContext)
		return recorder
	}

	assertContainsOnly := func(name string, recorder *httptest.ResponseRecorder, allowedIDs ...string) {
		t.Helper()
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", name, recorder.Code, recorder.Body.String())
		}
		for _, allowed := range allowedIDs {
			if !strings.Contains(recorder.Body.String(), allowed) {
				t.Fatalf("%s missing %q: %s", name, allowed, recorder.Body.String())
			}
		}
		if strings.Contains(recorder.Body.String(), "forbidden-home-model") {
			t.Fatalf("%s leaked a forbidden Home model: %s", name, recorder.Body.String())
		}
	}
	openAI := invoke("/v1/models", nil, "", server.handleHomeModels)
	if openAI.Code != http.StatusOK {
		t.Fatalf("OpenAI status=%d body=%s commands=%v", openAI.Code, openAI.Body.String(), commands.snapshot())
	}
	assertContainsOnly("OpenAI", openAI, "shared-policy-model", "smart-policy-model")
	assertContainsOnly("Codex", invoke("/v1/models?client_version=0.1", nil, "", func(c *gin.Context) {
		server.handleHomeCodexClientModels(c, c.Query("client_version"))
	}), "shared-policy-model", "smart-policy-model")
	assertContainsOnly("Claude", invoke("/v1/models", map[string]string{"Anthropic-Version": "2023-06-01"}, "", server.handleHomeModels), "claude-fable-5-dd-")
	assertContainsOnly("Gemini", invoke("/v1beta/models", nil, "", server.handleHomeGeminiModels), "models/shared-policy-model", "models/smart-policy-model")
	assertContainsOnly("Grok", invoke("/v1/models", map[string]string{"User-Agent": "grok-shell/0.2.119"}, "", server.handleGrokModels), "shared-policy-model", "smart-policy-model")

	alias := invoke("/v1beta/models/smart-policy-model", nil, "/smart-policy-model", server.handleHomeGeminiModel)
	assertContainsOnly("Gemini single alias", alias, "models/smart-policy-model", "Shared Home Model")
	forbidden := invoke("/v1beta/models/forbidden-home-model", nil, "/forbidden-home-model", server.handleHomeGeminiModel)
	if forbidden.Code != http.StatusNotFound || strings.Contains(forbidden.Body.String(), "Forbidden Home Model") {
		t.Fatalf("Gemini single forbidden status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
}

type apiKeyPolicyRedisCommandLog struct {
	mu       sync.Mutex
	commands [][]string
}

func (l *apiKeyPolicyRedisCommandLog) append(args []string) {
	l.mu.Lock()
	l.commands = append(l.commands, append([]string(nil), args...))
	l.mu.Unlock()
}

func (l *apiKeyPolicyRedisCommandLog) snapshot() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([][]string, len(l.commands))
	for index := range l.commands {
		out[index] = append([]string(nil), l.commands[index]...)
	}
	return out
}

func newAPIKeyPolicyHomeModelsClient(t *testing.T, payload []byte) (*home.Client, *apiKeyPolicyRedisCommandLog) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var handlers sync.WaitGroup
	commands := &apiKeyPolicyRedisCommandLog{}
	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			handlers.Add(1)
			go func(conn net.Conn) {
				defer handlers.Done()
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					args, readErr := readAPIKeyPolicyRedisCommand(reader)
					if readErr != nil {
						return
					}
					commands.append(args)
					response := "+OK\r\n"
					if len(args) > 0 {
						switch strings.ToUpper(args[0]) {
						case "HELLO":
							response = "%6\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"
						case "GET":
							response = "$" + strconv.Itoa(len(payload)) + "\r\n" + string(payload) + "\r\n"
						}
					}
					if _, writeErr := io.WriteString(conn, response); writeErr != nil {
						return
					}
				}
			}(conn)
		}
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := home.New(config.HomeConfig{Enabled: true, Host: host, Port: port, DisableClusterDiscovery: true})
	t.Cleanup(func() {
		client.Close()
		_ = listener.Close()
		<-done
		handlers.Wait()
	})
	return client, commands
}

func readAPIKeyPolicyRedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("expected RESP array, got %q", line)
	}
	count, err := strconv.Atoi(strings.TrimPrefix(line, "*"))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for index := 0; index < count; index++ {
		bulk, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		size, sizeErr := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(bulk), "$"))
		if sizeErr != nil {
			return nil, sizeErr
		}
		raw := make([]byte, size+2)
		if _, readErr = io.ReadFull(reader, raw); readErr != nil {
			return nil, readErr
		}
		args = append(args, string(raw[:size]))
	}
	return args, nil
}

func TestAPIKeyPolicyModelDiscoveryUnavailableDoesNotLeakCatalog(t *testing.T) {
	server, service := newAPIKeyPolicyModelsServer(t)
	service.MarkUnavailable()
	for _, test := range []struct {
		name    string
		path    string
		headers map[string]string
		assert  func(*testing.T, map[string]any)
	}{
		{name: "OpenAI", path: "/v1/models", assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if errorBody["type"] != "server_error" || errorBody["code"] != "api_key_policy_unavailable" {
				t.Fatalf("OpenAI envelope=%#v", body)
			}
		}},
		{name: "Claude", path: "/v1/models", headers: map[string]string{"Anthropic-Version": "2023-06-01"}, assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if body["type"] != "error" || errorBody["type"] != "api_error" || !strings.Contains(errorBody["message"].(string), "api_key_policy_unavailable") {
				t.Fatalf("Claude envelope=%#v", body)
			}
		}},
		{name: "Gemini", path: "/v1beta/models", assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if errorBody["code"] != float64(http.StatusServiceUnavailable) || errorBody["status"] != "UNAVAILABLE" || errorBody["reason"] != "api_key_policy_unavailable" {
				t.Fatalf("Gemini envelope=%#v", body)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := requestAPIKeyPolicyModels(t, server, test.path, test.headers)
			if recorder.Code != http.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "shared-policy-model") || !strings.Contains(recorder.Body.String(), "api_key_policy_unavailable") {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			test.assert(t, body)
		})
	}
}

func TestAPIKeyPolicyForbiddenExecutionUsesNativeProtocolEnvelopes(t *testing.T) {
	server, _ := newAPIKeyPolicyModelsServer(t)
	for _, test := range []struct {
		name    string
		path    string
		headers map[string]string
		body    string
		assert  func(*testing.T, map[string]any)
	}{
		{name: "OpenAI", path: "/v1/chat/completions", body: `{"model":"forbidden-model","messages":[]}`, assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if errorBody["type"] != "permission_error" || errorBody["code"] != "profile_model_forbidden" {
				t.Fatalf("OpenAI envelope=%#v", body)
			}
		}},
		{name: "Claude", path: "/v1/messages", headers: map[string]string{"Anthropic-Version": "2023-06-01"}, body: `{"model":"forbidden-model","messages":[],"max_tokens":1}`, assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if body["type"] != "error" || errorBody["type"] != "permission_error" || !strings.Contains(errorBody["message"].(string), "profile_model_forbidden") {
				t.Fatalf("Claude envelope=%#v", body)
			}
		}},
		{name: "Gemini", path: "/v1beta/models/forbidden-model:generateContent", body: `{"contents":[]}`, assert: func(t *testing.T, body map[string]any) {
			errorBody, _ := body["error"].(map[string]any)
			if errorBody["code"] != float64(http.StatusForbidden) || errorBody["status"] != "PERMISSION_DENIED" || errorBody["reason"] != "profile_model_forbidden" {
				t.Fatalf("Gemini envelope=%#v", body)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			req.Header.Set("Authorization", "Bearer test-key")
			req.Header.Set("Content-Type", "application/json")
			for key, value := range test.headers {
				req.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			server.engine.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "profile_model_forbidden") {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			test.assert(t, body)
		})
	}
}

func equalStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	set := make(map[string]int, len(left))
	for _, value := range left {
		set[value]++
	}
	for _, value := range right {
		if set[value] == 0 {
			return false
		}
		set[value]--
	}
	return true
}
