package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementRouteCaller interface {
	CallManagement(context.Context, pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error)
}

type pluginUsageCursor struct {
	LatestID   int64 `json:"latest_id"`
	Generation int64 `json:"generation"`
}

func pollPluginUsageEvents(ctx context.Context, caller managementRouteCaller, afterID int64) ([]byte, pluginUsageCursor, error) {
	if caller == nil {
		return nil, pluginUsageCursor{}, fmt.Errorf("plugin host is not available")
	}
	query := make(url.Values)
	query.Set("after_id", strconv.FormatInt(max(afterID, 0), 10))
	query.Set("limit", "100")
	resp, handled, err := caller.CallManagement(ctx, pluginapi.ManagementRequest{
		Method: http.MethodGet, Path: "/v0/management/usage/events", Query: query,
	})
	if err != nil {
		return nil, pluginUsageCursor{}, err
	}
	if !handled {
		return nil, pluginUsageCursor{}, fmt.Errorf("plugin usage events route is not available")
	}
	if resp.StatusCode != 0 && resp.StatusCode != http.StatusOK {
		return nil, pluginUsageCursor{}, fmt.Errorf("plugin usage events returned status %d", resp.StatusCode)
	}
	var cursor pluginUsageCursor
	if err := json.Unmarshal(resp.Body, &cursor); err != nil {
		return nil, pluginUsageCursor{}, fmt.Errorf("decode plugin usage events: %w", err)
	}
	return resp.Body, cursor, nil
}

func (s *Server) servePluginUsageStream(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
		return
	}
	lastID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("after_id")), 10, 64)
	clientGeneration, _ := strconv.ParseInt(strings.TrimSpace(c.Query("generation")), 10, 64)
	body, cursor, err := pollPluginUsageEvents(c.Request.Context(), s.pluginHost, lastID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	writeEvent := func(name string, data []byte) bool {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if clientGeneration > 0 && cursor.Generation > 0 && clientGeneration != cursor.Generation {
		writeEvent("reset", body)
		return
	}
	if cursor.LatestID > lastID {
		lastID = cursor.LatestID
		if !writeEvent("usage", body) {
			return
		}
	} else if !writeEvent("ready", body) {
		return
	}
	currentGeneration := cursor.Generation
	pollTicker := time.NewTicker(time.Second)
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer pollTicker.Stop()
	defer keepaliveTicker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-keepaliveTicker.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-pollTicker.C:
			body, cursor, err = pollPluginUsageEvents(c.Request.Context(), s.pluginHost, lastID)
			if err != nil {
				return
			}
			if currentGeneration > 0 && cursor.Generation > 0 && cursor.Generation != currentGeneration {
				writeEvent("reset", body)
				return
			}
			if cursor.Generation > 0 {
				currentGeneration = cursor.Generation
			}
			if cursor.LatestID <= lastID {
				continue
			}
			lastID = cursor.LatestID
			if !writeEvent("usage", body) {
				return
			}
		}
	}
}
