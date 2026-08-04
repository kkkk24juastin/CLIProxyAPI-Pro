package management

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestManagementPanelSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management.html")
	data := []byte("management panel")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	got, err := managementPanelSHA256(path)
	if err != nil {
		t.Fatalf("managementPanelSHA256() error = %v", err)
	}
	if got != want {
		t.Fatalf("managementPanelSHA256() = %q, want %q", got, want)
	}
}

func TestPostCheckManagementPanelUpdateRequiresConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/management-panel/check-update", nil)

	(&Handler{}).PostCheckManagementPanelUpdate(context)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
