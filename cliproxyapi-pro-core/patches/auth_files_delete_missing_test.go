package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type missingSpoolDeleteStore struct {
	deleted []string
}

func (s *missingSpoolDeleteStore) List(context.Context) ([]*coreauth.Auth, error) {
	return nil, nil
}

func (s *missingSpoolDeleteStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}

func (s *missingSpoolDeleteStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *missingSpoolDeleteStore) SetBaseDir(string) {}

func registerMissingSpoolAuth(t *testing.T, manager *coreauth.Manager, authDir, fileName string) *coreauth.Auth {
	t.Helper()
	record := &coreauth.Auth{
		ID:       fileName,
		FileName: fileName,
		Provider: "antigravity",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path": filepath.Join(authDir, fileName),
		},
		Metadata: map[string]any{"type": "antigravity"},
	}
	if _, err := manager.Register(context.Background(), record); err != nil {
		t.Fatalf("register auth: %v", err)
	}
	return record
}

func TestDeleteAuthFile_MissingSpoolStillDeletesStoreAndRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authDir := t.TempDir()
	fileName := "missing-spool.json"
	manager := coreauth.NewManager(nil, nil, nil)
	record := registerMissingSpoolAuth(t, manager, authDir, fileName)
	store := &missingSpoolDeleteStore{}
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodDelete,
		"/v0/management/auth-files?name="+url.QueryEscape(fileName),
		nil,
	)
	h.DeleteAuthFile(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != filepath.Join(authDir, fileName) {
		t.Fatalf("store deletes = %#v, want missing spool path", store.deleted)
	}
	if _, ok := manager.GetByID(record.ID); ok {
		t.Fatalf("runtime auth %q was not removed", record.ID)
	}
}

func TestDeleteAllAuthFiles_IncludesMissingSpoolEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authDir := t.TempDir()
	manager := coreauth.NewManager(nil, nil, nil)
	records := []*coreauth.Auth{
		registerMissingSpoolAuth(t, manager, authDir, "missing-a.json"),
		registerMissingSpoolAuth(t, manager, authDir, "missing-b.json"),
	}
	store := &missingSpoolDeleteStore{}
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = store

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/auth-files?all=true", nil)
	h.DeleteAuthFile(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("delete all status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if len(store.deleted) != len(records) {
		t.Fatalf("store delete count = %d, want %d", len(store.deleted), len(records))
	}
	for _, record := range records {
		if _, ok := manager.GetByID(record.ID); ok {
			t.Fatalf("runtime auth %q was not removed", record.ID)
		}
	}
}
