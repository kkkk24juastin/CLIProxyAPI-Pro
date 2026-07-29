package pluginhost

import (
	"bytes"
	"context"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// CallManagement invokes a registered plugin route without buffering it through an HTTP writer.
// Host-owned transport bridges use this for bounded polling requests.
func (h *Host) CallManagement(ctx context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error) {
	if h == nil {
		return pluginapi.ManagementResponse{}, false, nil
	}
	key := managementRouteKey(req.Method, req.Path)
	h.mu.Lock()
	record, ok := h.managementRoutes[key]
	h.mu.Unlock()
	if !ok || record.route.Handler == nil || h.isPluginFused(record.pluginID) {
		return pluginapi.ManagementResponse{}, false, nil
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.Path = strings.TrimSpace(req.Path)
	req.Headers = cloneHeader(req.Headers)
	req.Query = cloneValues(req.Query)
	req.Body = bytes.Clone(req.Body)
	resp, err := h.callManagementHandler(ctx, record, req)
	return resp, true, err
}
