package proxyutil

import (
	"strings"
	"sync"
)

var runtimeProxyOverride struct {
	sync.RWMutex
	enabled   bool
	base      string
	effective string
}

// SetRuntimeProxyOverride replaces only the configured global proxy value at
// transport construction time. Credential-level proxy URLs and explicit
// "direct" settings remain untouched.
func SetRuntimeProxyOverride(base, effective string) {
	runtimeProxyOverride.Lock()
	runtimeProxyOverride.base = strings.TrimSpace(base)
	runtimeProxyOverride.effective = strings.TrimSpace(effective)
	runtimeProxyOverride.enabled = runtimeProxyOverride.effective != ""
	runtimeProxyOverride.Unlock()
}

func ClearRuntimeProxyOverride() {
	runtimeProxyOverride.Lock()
	runtimeProxyOverride.enabled = false
	runtimeProxyOverride.base = ""
	runtimeProxyOverride.effective = ""
	runtimeProxyOverride.Unlock()
}

func resolveRuntimeProxyOverride(raw string) string {
	runtimeProxyOverride.RLock()
	enabled := runtimeProxyOverride.enabled
	base := runtimeProxyOverride.base
	effective := runtimeProxyOverride.effective
	runtimeProxyOverride.RUnlock()
	if enabled && strings.TrimSpace(raw) == base {
		return effective
	}
	return raw
}
