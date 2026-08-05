package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type authFileConnectionExecutor struct {
	lastAuthID string
	lastModel  string
	response   []byte
	err        error
}

func (e *authFileConnectionExecutor) Identifier() string { return "codex" }

func (e *authFileConnectionExecutor) Execute(_ context.Context, auth *coreauth.Auth, req coreexecutor.Request, _ coreexecutor.Options) (coreexecutor.Response, error) {
	if auth != nil {
		e.lastAuthID = auth.ID
	}
	e.lastModel = req.Model
	return coreexecutor.Response{Payload: e.response}, e.err
}

func (e *authFileConnectionExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, nil
}

func (e *authFileConnectionExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *authFileConnectionExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *authFileConnectionExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestAuthFileConnectionUsesExactAuthAndReturnsOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	executor := &authFileConnectionExecutor{response: []byte(`{"choices":[{"message":{"content":"OK"}}]}`)}
	manager.RegisterExecutor(executor)
	auth, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "auth-1",
		Provider: "codex",
		FileName: "account.json",
		Status:   coreauth.StatusDisabled,
		Disabled: true,
	})
	if errRegister != nil {
		t.Fatalf("Register() error = %v", errRegister)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, "codex", []*registry.ModelInfo{{ID: "gpt-test", Type: "openai"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(auth.ID) })

	handler := &Handler{authManager: manager}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/test", strings.NewReader(`{"name":"account.json","auth_index":"`+auth.EnsureIndex()+`","model":"gpt-test"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.TestAuthFileConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if executor.lastAuthID != auth.ID || executor.lastModel != "gpt-test" {
		t.Fatalf("executor received auth=%q model=%q", executor.lastAuthID, executor.lastModel)
	}
	if updated, ok := manager.GetByID(auth.ID); !ok || updated == nil || !updated.Disabled {
		t.Fatalf("connection test changed the operator-disabled state: %#v", updated)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"success":true`) || !strings.Contains(body, `"output":"OK"`) {
		t.Fatalf("response body = %s", body)
	}
}

func TestAuthFileConnectionRejectsUnsupportedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(&authFileConnectionExecutor{})
	auth, errRegister := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "auth-unsupported-model",
		Provider: "codex",
		FileName: "account.json",
		Status:   coreauth.StatusActive,
	})
	if errRegister != nil {
		t.Fatalf("Register() error = %v", errRegister)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, "codex", []*registry.ModelInfo{{ID: "gpt-test", Type: "openai"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(auth.ID) })

	handler := &Handler{authManager: manager}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/test", strings.NewReader(`{"name":"account.json","model":"image-model"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.TestAuthFileConnection(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestExtractAuthFileConnectionOutput(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "chat completions", payload: `{"choices":[{"message":{"content":"OK"}}]}`, want: "OK"},
		{name: "responses", payload: `{"output":[{"content":[{"type":"output_text","text":"ready"}]}]}`, want: "ready"},
		{name: "gemini", payload: `{"candidates":[{"content":{"parts":[{"text":"connected"}]}}]}`, want: "connected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := extractAuthFileConnectionOutput([]byte(test.payload)); got != test.want {
				t.Fatalf("extractAuthFileConnectionOutput() = %q, want %q", got, test.want)
			}
		})
	}
}
