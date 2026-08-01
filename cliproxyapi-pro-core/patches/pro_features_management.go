package management

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy/config"
	proapp "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/app"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/config"
)

func (h *Handler) SetProApp(application *proapp.App) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.proApp = application
	h.mu.Unlock()
}

func (h *Handler) proApplication() *proapp.App {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.proApp
}

func (h *Handler) RegisterProFeatureRoutes(group *gin.RouterGroup) {
	if h == nil || group == nil {
		return
	}
	group.GET("/pro/proxy-pool/config", h.GetProxyPoolConfig)
	group.PUT("/pro/proxy-pool/config", h.PutProxyPoolConfig)
	group.PATCH("/pro/proxy-pool/config", h.PutProxyPoolConfig)
	group.GET("/pro/proxy-pool/status", h.GetProxyPoolStatus)
	group.POST("/pro/proxy-pool/test", h.TestProxyPoolNode)
	group.POST("/pro/proxy-pool/test-all", h.TestAllProxyPoolNodes)
	group.POST("/pro/proxy-pool/recover", h.RecoverProxyPoolNode)
	group.POST("/pro/proxy-pool/reset-stats", h.ResetProxyPoolStats)

	group.GET("/pro/oauth-model-policy/config", h.GetOAuthModelPolicyConfig)
	group.PUT("/pro/oauth-model-policy/config", h.PutOAuthModelPolicyConfig)
	group.PATCH("/pro/oauth-model-policy/config", h.PutOAuthModelPolicyConfig)
	group.GET("/pro/oauth-model-policy/status", h.GetOAuthModelPolicyStatus)
}

func unavailableProApp(c *gin.Context) {
	if c == nil {
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pro_features_unavailable"})
}

func (h *Handler) GetProxyPoolConfig(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	c.JSON(http.StatusOK, runtime.ProxyConfig())
}

func (h *Handler) PutProxyPoolConfig(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	cfg := proxyconfig.Default()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_proxy_pool_config", "message": err.Error()})
		return
	}
	if err := runtime.UpdateProxyConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_proxy_pool_config", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": runtime.ProxyConfig(), "status": runtime.ProxyStatus()})
}

func (h *Handler) GetProxyPoolStatus(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	c.JSON(http.StatusOK, runtime.ProxyStatus())
}

func (h *Handler) TestProxyPoolNode(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
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
	result := runtime.ProbeProxy(ctx, body.NodeID, body.ProxyURL, body.URL)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadGateway
	}
	c.JSON(status, result)
}

func (h *Handler) TestAllProxyPoolNodes(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	var body struct {
		Concurrency int `json:"concurrency"`
	}
	_ = c.ShouldBindJSON(&body)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	c.JSON(http.StatusOK, gin.H{"results": runtime.ProbeAllProxies(ctx, body.Concurrency)})
}

func (h *Handler) RecoverProxyPoolNode(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	var body struct {
		NodeID string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.NodeID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "node_id is required"})
		return
	}
	if err := runtime.RecoverProxy(body.NodeID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ResetProxyPoolStats(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	runtime.ResetProxyStats()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) GetOAuthModelPolicyConfig(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	raw, err := modelconfig.Marshal(runtime.ModelConfig())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) PutOAuthModelPolicyConfig(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil || len(raw) == 0 || !json.Valid(raw) {
		if err == nil {
			err = errors.New("request body must contain valid JSON")
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_oauth_model_policy_config", "message": err.Error()})
		return
	}
	cfg, err := modelconfig.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_oauth_model_policy_config", "message": err.Error()})
		return
	}
	if err := runtime.UpdateModelConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_oauth_model_policy_config", "message": err.Error()})
		return
	}
	normalized, err := modelconfig.Marshal(runtime.ModelConfig())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var responseConfig json.RawMessage = normalized
	c.JSON(http.StatusOK, gin.H{"config": responseConfig, "status": runtime.ModelStatus()})
}

func (h *Handler) GetOAuthModelPolicyStatus(c *gin.Context) {
	runtime := h.proApplication()
	if runtime == nil {
		unavailableProApp(c)
		return
	}
	c.JSON(http.StatusOK, runtime.ModelStatus())
}
