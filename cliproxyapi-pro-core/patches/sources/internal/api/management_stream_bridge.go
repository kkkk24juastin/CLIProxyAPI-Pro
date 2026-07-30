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
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type pluginManagementEventPage struct {
	Sequence int64             `json:"sequence"`
	Messages []json.RawMessage `json:"messages"`
}

var pluginManagementStreamUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

var pluginManagementEventQueryKeys = map[string]struct{}{
	"details": {}, "result_limit": {}, "log_limit": {}, "result_page": {}, "result_page_size": {},
	"result_filter": {}, "result_pending_only": {}, "result_provider": {}, "result_search": {},
	"log_page": {}, "log_page_size": {}, "log_level": {},
}

func sanitizedPluginManagementEventQuery(source url.Values) url.Values {
	query := make(url.Values)
	for key, values := range source {
		if _, ok := pluginManagementEventQueryKeys[key]; ok {
			query[key] = append([]string(nil), values...)
		}
	}
	return query
}

func pollPluginManagementEvents(ctx context.Context, caller managementRouteCaller, path string, baseQuery url.Values, after int64) (pluginManagementEventPage, error) {
	if caller == nil {
		return pluginManagementEventPage{}, fmt.Errorf("plugin host is not available")
	}
	query := make(url.Values, len(baseQuery)+1)
	for key, values := range baseQuery {
		query[key] = append([]string(nil), values...)
	}
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

func pluginManagementWebSocketResponseHeader(request *http.Request) http.Header {
	header := make(http.Header)
	if request == nil {
		return header
	}
	for _, protocol := range request.Header.Values("Sec-WebSocket-Protocol") {
		for _, candidate := range strings.Split(protocol, ",") {
			candidate = strings.TrimSpace(candidate)
			if strings.HasPrefix(candidate, "cpa-management.") {
				header.Set("Sec-WebSocket-Protocol", candidate)
				return header
			}
		}
	}
	return header
}

func (s *Server) servePluginManagementWebSocket(c *gin.Context, eventPath string) {
	connection, err := pluginManagementStreamUpgrader.Upgrade(c.Writer, c.Request, pluginManagementWebSocketResponseHeader(c.Request))
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
	eventQuery := sanitizedPluginManagementEventQuery(c.Request.URL.Query())
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
	page, err := pollPluginManagementEvents(c.Request.Context(), s.pluginHost, eventPath, eventQuery, sequence)
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
			page, err = pollPluginManagementEvents(c.Request.Context(), s.pluginHost, eventPath, eventQuery, sequence)
			if err != nil || !writePage(page) {
				return
			}
		}
	}
}

func (s *Server) servePluginAccountInspectionStream(c *gin.Context) {
	s.servePluginManagementWebSocket(c, "/v0/management/account-inspection/events")
}
