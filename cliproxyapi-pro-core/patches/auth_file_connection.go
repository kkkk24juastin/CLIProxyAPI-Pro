package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

const authFileConnectionTestTimeout = 45 * time.Second

type authFileConnectionTestRequest struct {
	Name      string `json:"name"`
	AuthIndex string `json:"auth_index"`
	Model     string `json:"model"`
}

type authFileConnectionTestResponse struct {
	Success    bool   `json:"success"`
	Model      string `json:"model,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// TestAuthFileConnection sends a minimal real model request through one exact
// auth file. The pinned auth ID prevents a healthy credential from hiding a
// failure on the card the operator selected.
func (h *Handler) TestAuthFileConnection(c *gin.Context) {
	var body authFileConnectionTestRequest
	if errBind := c.ShouldBindJSON(&body); errBind != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.AuthIndex = strings.TrimSpace(body.AuthIndex)
	body.Model = strings.TrimSpace(body.Model)
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if h == nil || h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth manager is unavailable"})
		return
	}

	auth, ok := h.lookupAuthFile(body.Name, body.AuthIndex)
	if !ok || auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth file not found"})
		return
	}
	model, errModel := resolveAuthFileConnectionTestModel(auth.ID, body.Model)
	if errModel != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errModel.Error()})
		return
	}

	payload, errPayload := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{{
			"role":    "user",
			"content": "Reply with OK only.",
		}},
		"stream": false,
	})
	if errPayload != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build test request"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), authFileConnectionTestTimeout)
	defer cancel()
	startedAt := time.Now()
	response, errExecute := h.authManager.ExecutePinnedAuth(
		ctx,
		auth.ID,
		coreexecutor.Request{
			Model:   model,
			Payload: payload,
			Format:  sdktranslator.FormatOpenAI,
		},
		coreexecutor.Options{
			OriginalRequest: payload,
			SourceFormat:    sdktranslator.FormatOpenAI,
			ResponseFormat:  sdktranslator.FormatOpenAI,
			Metadata: map[string]any{
				coreexecutor.PinnedAuthMetadataKey:     auth.ID,
				coreexecutor.RequestedModelMetadataKey: model,
				coreexecutor.GenerateMetadataKey:       true,
			},
		},
	)
	latencyMS := time.Since(startedAt).Milliseconds()
	if errExecute != nil {
		message, code, status := authFileConnectionTestError(errExecute)
		c.JSON(http.StatusOK, authFileConnectionTestResponse{
			Success:    false,
			Model:      model,
			LatencyMS:  latencyMS,
			Error:      message,
			ErrorCode:  code,
			HTTPStatus: status,
		})
		return
	}

	output := extractAuthFileConnectionOutput(response.Payload)
	if output == "" {
		c.JSON(http.StatusOK, authFileConnectionTestResponse{
			Success:   false,
			Model:     model,
			LatencyMS: latencyMS,
			Error:     "upstream completed without text output",
			ErrorCode: "empty_output",
		})
		return
	}

	c.JSON(http.StatusOK, authFileConnectionTestResponse{
		Success:   true,
		Model:     model,
		LatencyMS: latencyMS,
		Output:    output,
	})
}

func resolveAuthFileConnectionTestModel(authID string, requested string) (string, error) {
	models := registry.GetGlobalRegistry().GetModelsForClient(strings.TrimSpace(authID))
	textModels := make([]string, 0, len(models))
	for _, model := range models {
		if model == nil || !isAuthFileConnectionTextModel(model) {
			continue
		}
		textModels = append(textModels, strings.TrimSpace(model.ID))
	}
	if len(textModels) == 0 {
		return "", fmt.Errorf("auth file has no text model available for testing")
	}
	if requested == "" {
		return textModels[0], nil
	}
	for _, model := range textModels {
		if strings.EqualFold(model, requested) {
			return model, nil
		}
	}
	return "", fmt.Errorf("auth file does not support test model %q", requested)
}

func isAuthFileConnectionTextModel(model *registry.ModelInfo) bool {
	if model == nil {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(model.ID))
	if id == "" || strings.Contains(id, "image") || strings.Contains(id, "video") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(model.Type), registry.OpenAIImageModelType) {
		return false
	}
	if len(model.SupportedOutputModalities) == 0 {
		return true
	}
	for _, modality := range model.SupportedOutputModalities {
		if strings.EqualFold(strings.TrimSpace(modality), "text") {
			return true
		}
	}
	return false
}

func authFileConnectionTestError(err error) (message string, code string, status int) {
	if err == nil {
		return "connection test failed", "connection_test_failed", 0
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "connection test timed out", "timeout", http.StatusGatewayTimeout
	}
	if errors.Is(err, context.Canceled) {
		return "connection test was canceled", "canceled", 0
	}
	message = strings.TrimSpace(err.Error())
	if message == "" {
		message = "connection test failed"
	}
	var authErr *coreauth.Error
	if errors.As(err, &authErr) && authErr != nil {
		code = strings.TrimSpace(authErr.Code)
		status = authErr.HTTPStatus
	}
	var statusErr coreexecutor.StatusError
	if status == 0 && errors.As(err, &statusErr) && statusErr != nil {
		status = statusErr.StatusCode()
	}
	if code == "" {
		code = "connection_test_failed"
	}
	return message, code, status
}

func extractAuthFileConnectionOutput(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var decoded map[string]any
	if errUnmarshal := json.Unmarshal(payload, &decoded); errUnmarshal != nil {
		return strings.TrimSpace(string(payload))
	}
	if output := stringField(decoded, "output_text"); output != "" {
		return output
	}
	if choices, ok := decoded["choices"].([]any); ok {
		for _, rawChoice := range choices {
			choice, okChoice := rawChoice.(map[string]any)
			if !okChoice {
				continue
			}
			if message, okMessage := choice["message"].(map[string]any); okMessage {
				if output := connectionContentText(message["content"]); output != "" {
					return output
				}
			}
			if output := stringField(choice, "text"); output != "" {
				return output
			}
		}
	}
	if output := connectionContentText(decoded["output"]); output != "" {
		return output
	}
	if output := connectionContentText(decoded["content"]); output != "" {
		return output
	}
	if candidates, ok := decoded["candidates"].([]any); ok {
		for _, rawCandidate := range candidates {
			candidate, okCandidate := rawCandidate.(map[string]any)
			if !okCandidate {
				continue
			}
			if content, okContent := candidate["content"].(map[string]any); okContent {
				if output := connectionContentText(content["parts"]); output != "" {
					return output
				}
			}
		}
	}
	return ""
}

func connectionContentText(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		if text := stringField(value, "text"); text != "" {
			return text
		}
		if text := stringField(value, "output_text"); text != "" {
			return text
		}
		for _, key := range []string{"content", "parts"} {
			if text := connectionContentText(value[key]); text != "" {
				return text
			}
		}
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := connectionContentText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	}
	return ""
}

func stringField(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
