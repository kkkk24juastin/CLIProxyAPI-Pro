package management

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/managementasset"
)

// PostCheckManagementPanelUpdate checks the latest management.html and applies it when its
// release digest differs from the local asset. The updater's normal throttle remains active.
func (h *Handler) PostCheckManagementPanelUpdate(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "handler_unavailable"})
		return
	}

	h.mu.Lock()
	cfg := h.cfg
	configFilePath := h.configFilePath
	proxyURL := ""
	panelRepository := ""
	if cfg != nil {
		proxyURL = cfg.ProxyURL
		panelRepository = cfg.RemoteManagement.PanelGitHubRepository
	}
	h.mu.Unlock()
	if cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config_unavailable"})
		return
	}

	staticDir := managementasset.StaticDir(configFilePath)
	filePath := managementasset.FilePath(configFilePath)
	beforeHash, beforeErr := managementPanelSHA256(filePath)
	if beforeErr != nil && !errors.Is(beforeErr, os.ErrNotExist) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "management_panel_read_failed",
			"message": beforeErr.Error(),
		})
		return
	}

	available := managementasset.EnsureLatestManagementHTML(
		c.Request.Context(),
		staticDir,
		proxyURL,
		panelRepository,
	)
	if !available {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "management_panel_update_failed",
			"message": "management panel asset is unavailable after update check",
		})
		return
	}

	afterHash, err := managementPanelSHA256(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "management_panel_read_failed",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "checked",
		"updated": beforeHash != afterHash,
		"sha256":  afterHash,
	})
}

func managementPanelSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
