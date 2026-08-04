package host

import "github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"

// ProxyOverride is the only adapter allowed to mutate the upstream-wide
// transport override used by the built-in provider executors.
type ProxyOverride struct{}

func NewProxyOverride() ProxyOverride { return ProxyOverride{} }

func (ProxyOverride) Set(baseProxyURL, targetProxyURL string) {
	proxyutil.SetRuntimeProxyOverride(baseProxyURL, targetProxyURL)
}

func (ProxyOverride) Clear() { proxyutil.ClearRuntimeProxyOverride() }
