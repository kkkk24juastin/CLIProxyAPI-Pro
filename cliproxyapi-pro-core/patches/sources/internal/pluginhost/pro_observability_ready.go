package pluginhost

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

// ProObservabilityReady verifies the complete persistence capability set before
// the proxy service starts. A same-named or partially loaded plugin must not
// satisfy the fail-closed startup gate.
func (h *Host) ProObservabilityReady(id string) bool {
	if h == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, record := range h.activeRecords() {
		if record.id != id || h.isPluginFused(record.id) {
			continue
		}
		caps := record.plugin.Capabilities
		return caps.UsagePlugin != nil && caps.ManagementAPI != nil && caps.Scheduler != nil &&
			caps.QuotaCacheStore != nil && caps.RuntimeStateStore != nil && caps.ProSettingsStore != nil
	}
	return false
}

func detachProObservabilityBackends(host *Host) {
	if host == nil || !host.PluginRegistered(proObservabilityPluginID) {
		return
	}
	// Runtime writes must drain while the dynamic library is still callable and
	// the host snapshot still exposes its capability facade.
	embeddedusage.SetRuntimeStatePluginBackend(nil)
	embeddedusage.SetQuotaCachePluginBackend(nil)
	embeddedusage.SetProSettingsPluginBackend(nil)
}
