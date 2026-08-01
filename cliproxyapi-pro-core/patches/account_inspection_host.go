package management

import (
	"context"
	"fmt"

	proinspection "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/inspection"
)

// accountInspectionQuotaAdapter is the host-side implementation of the
// inspection module's quota port. Keeping it outside plugin_quota.go prevents
// the standalone quota endpoint from depending back on inspection.
type accountInspectionQuotaAdapter struct{ h *Handler }

func (a accountInspectionQuotaAdapter) FetchQuota(ctx context.Context, authIndex string) (proinspection.QuotaResult, error) {
	if a.h == nil {
		return proinspection.QuotaResult{}, fmt.Errorf("management handler unavailable")
	}
	auth := a.h.authByIndex(authIndex)
	if auth == nil {
		return proinspection.QuotaResult{}, fmt.Errorf("auth not found")
	}
	result, serviceStatus, _, err := a.h.fetchAndPersistPluginQuota(ctx, auth)
	return proinspection.QuotaResult{
		Snapshot:       result.Snapshot,
		ServiceStatus:  serviceStatus,
		UpstreamStatus: result.UpstreamStatus,
	}, err
}
