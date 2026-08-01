package inspection

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// QuotaResult is the provider-neutral quota projection needed by inspection.
// Plugin-host and Auth Manager details remain in the Management adapter.
type QuotaResult struct {
	Snapshot       pluginapi.QuotaSnapshot
	ServiceStatus  int
	UpstreamStatus int
}

// QuotaGateway lets inspection probe a normalized provider quota by stable
// auth index without depending on a Management Handler private method.
type QuotaGateway interface {
	FetchQuota(context.Context, string) (QuotaResult, error)
}
