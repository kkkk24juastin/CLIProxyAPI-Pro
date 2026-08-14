package oauthpolicy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

func TestRefreshManagementRouteStartsPlanDetection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, err := New(context.Background(), &memorySettingsStore{items: map[string]settings.Item{}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	service.SetChangeHandler(func(context.Context) {
		close(started)
		<-release
	})
	router := gin.New()
	RegisterManagementRoutes(router.Group("/v0/management"), service)
	request := httptest.NewRequest(http.MethodPost, "/v0/management/pro/oauth-policy/refresh", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("plan detection refresh did not start")
	}
	if !service.Status().Refreshing {
		t.Fatal("refreshing status is false while refresh handler is running")
	}
	close(release)
}
