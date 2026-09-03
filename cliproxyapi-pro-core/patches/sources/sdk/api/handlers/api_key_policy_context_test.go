package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestGetContextWithCancelInheritsServerAPIKeyPolicySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	identity, errIdentity := apikeypolicy.NewAuthenticatedAPIKeyIdentity("server-authenticated-key")
	if errIdentity != nil {
		t.Fatal(errIdentity)
	}
	decision := apikeypolicy.PassthroughDecision()
	requestCtx := apikeypolicy.WithIdentity(context.Background(), identity)
	requestCtx = apikeypolicy.WithDecision(requestCtx, decision)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)

	handler := &BaseAPIHandler{Cfg: &config.SDKConfig{}}
	ctx, cancel := handler.GetContextWithCancel(nil, ginCtx, context.Background())
	defer cancel()
	gotIdentity, okIdentity := apikeypolicy.IdentityFromContext(ctx)
	gotDecision, okDecision := apikeypolicy.DecisionFromContext(ctx)
	if !okIdentity || gotIdentity.Hash() != identity.Hash() {
		t.Fatalf("identity inherited = %#v, %t", gotIdentity, okIdentity)
	}
	if !okDecision || gotDecision.Mode != apikeypolicy.ModePassthrough {
		t.Fatalf("decision inherited = %#v, %t", gotDecision, okDecision)
	}
}
