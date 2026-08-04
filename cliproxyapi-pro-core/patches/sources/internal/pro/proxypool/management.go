package proxypool

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/config"
)

func RegisterManagementRoutes(group *gin.RouterGroup, service *Service) {
	if group == nil {
		return
	}
	management := &managementHandler{service: service}
	group.GET("/pro/proxy-pool/config", management.getConfig)
	group.PUT("/pro/proxy-pool/config", management.putConfig)
	group.PATCH("/pro/proxy-pool/config", management.putConfig)
	group.GET("/pro/proxy-pool/status", management.getStatus)
	group.POST("/pro/proxy-pool/test", management.testNode)
	group.POST("/pro/proxy-pool/test-all", management.testAll)
	group.POST("/pro/proxy-pool/recover", management.recover)
	group.POST("/pro/proxy-pool/reset-stats", management.resetStats)
}

type managementHandler struct{ service *Service }

func (h *managementHandler) available(c *gin.Context) bool {
	if h != nil && h.service != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pro_features_unavailable"})
	return false
}

func (h *managementHandler) getConfig(c *gin.Context) {
	if h.available(c) {
		c.JSON(http.StatusOK, h.service.Config())
	}
}

func (h *managementHandler) putConfig(c *gin.Context) {
	if !h.available(c) {
		return
	}
	cfg := proxyconfig.Default()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_proxy_pool_config", "message": err.Error()})
		return
	}
	if err := h.service.UpdateConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_proxy_pool_config", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": h.service.Config(), "status": h.service.Status()})
}

func (h *managementHandler) getStatus(c *gin.Context) {
	if h.available(c) {
		c.JSON(http.StatusOK, h.service.Status())
	}
}

func (h *managementHandler) testNode(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var body struct {
		NodeID   string `json:"node_id"`
		ProxyURL string `json:"proxy_url"`
		URL      string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.NodeID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "node_id is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	result := h.service.Probe(ctx, body.NodeID, body.ProxyURL, body.URL)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadGateway
	}
	c.JSON(status, result)
}

func (h *managementHandler) testAll(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var body struct {
		Concurrency int `json:"concurrency"`
	}
	_ = c.ShouldBindJSON(&body)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	c.JSON(http.StatusOK, gin.H{"results": h.service.ProbeAll(ctx, body.Concurrency)})
}

func (h *managementHandler) recover(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var body struct {
		NodeID string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.NodeID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "node_id is required"})
		return
	}
	if err := h.service.Recover(body.NodeID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *managementHandler) resetStats(c *gin.Context) {
	if !h.available(c) {
		return
	}
	h.service.ResetStats()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
