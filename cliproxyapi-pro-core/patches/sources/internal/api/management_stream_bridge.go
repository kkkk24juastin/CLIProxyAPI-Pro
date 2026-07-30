package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type pluginManagementEventPage struct {
	Sequence int64             `json:"sequence"`
	Messages []json.RawMessage `json:"messages"`
}

var pluginManagementStreamUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func pollPluginManagementEvents(ctx context.Context, caller managementRouteCaller, path string, after int64) (pluginManagementEventPage, error) {
	if caller == nil {
		return pluginManagementEventPage{}, fmt.Errorf("plugin host is not available")
	}
	query := make(url.Values)
	query.Set("after_sequence", strconv.FormatInt(max(after, 0), 10))
	response, handled, err := caller.CallManagement(ctx, pluginapi.ManagementRequest{Method: http.MethodGet, Path: path, Query: query})
	if err != nil {
		return pluginManagementEventPage{}, err
	}
	if !handled {
		return pluginManagementEventPage{}, fmt.Errorf("plugin management event route is not available")
	}
	if response.StatusCode != 0 && response.StatusCode != http.StatusOK {
		return pluginManagementEventPage{}, fmt.Errorf("plugin management event route returned status %d", response.StatusCode)
	}
	var page pluginManagementEventPage
	if err = json.Unmarshal(response.Body, &page); err != nil {
		return page, fmt.Errorf("decode plugin management events: %w", err)
	}
	return page, nil
}

func (s *Server) servePluginManagementWebSocket(c *gin.Context, eventPath string) {
	connection, err := pluginManagementStreamUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	connection.SetReadLimit(4096)
	connection.SetPongHandler(func(string) error { return connection.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, readErr := connection.ReadMessage(); readErr != nil {
				return
			}
		}
	}()

	sequence := int64(0)
	poll := time.NewTicker(time.Second)
	ping := time.NewTicker(30 * time.Second)
	defer poll.Stop()
	defer ping.Stop()
	writePage := func(page pluginManagementEventPage) bool {
		if page.Sequence > sequence {
			sequence = page.Sequence
		}
		for _, message := range page.Messages {
			_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if errWrite := connection.WriteMessage(websocket.TextMessage, message); errWrite != nil {
				return false
			}
		}
		return true
	}
	page, err := pollPluginManagementEvents(c.Request.Context(), s.pluginHost, eventPath, sequence)
	if err != nil || !writePage(page) {
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-done:
			return
		case <-ping.C:
			_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err = connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-poll.C:
			page, err = pollPluginManagementEvents(c.Request.Context(), s.pluginHost, eventPath, sequence)
			if err != nil || !writePage(page) {
				return
			}
		}
	}
}

func (s *Server) servePluginAccountInspectionStream(c *gin.Context) {
	s.servePluginManagementWebSocket(c, "/v0/management/account-inspection/events")
}
