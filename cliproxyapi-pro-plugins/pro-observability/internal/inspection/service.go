package inspection

import (
	"context"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
	embeddedusage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage"
)

// Service owns the account-inspection scheduler, routes, persistence and
// lifecycle inside pro-observability.
type Service struct {
	ctx       context.Context
	cancel    context.CancelFunc
	gateway   Gateway
	scheduler *accountInspectionScheduler
	wg        sync.WaitGroup
}

// Handler is kept as a local alias so the migrated route implementation stays
// wire-compatible while no longer depending on Core's management.Handler.
type Handler = Service

func Start(ctx context.Context, gateway Gateway, config Config) (*Service, error) {
	if gateway == nil {
		return nil, fmt.Errorf("account inspection host gateway is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	service := &Service{ctx: serviceCtx, cancel: cancel, gateway: gateway}
	handler := newCompatHandler(serviceCtx, gateway, config)
	service.scheduler = newAccountInspectionScheduler(handler)
	embeddedusage.SetAccountInspectionScheduleHandlers(service.scheduler.exportSchedule, service.scheduler.importSchedule)
	embeddedusage.SetAccountInspectionSnapshotHandlers(service.scheduler.exportResultSnapshot, service.scheduler.importResultSnapshot)
	service.scheduler.cleanupLegacyQuotaCaches(context.Background())
	service.wg.Add(1)
	go func() {
		defer service.wg.Done()
		service.scheduler.loop(serviceCtx)
	}()
	return service, nil
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.scheduler != nil {
		s.scheduler.shutdown()
	}
	s.wg.Wait()
	embeddedusage.SetAccountInspectionScheduleHandlers(nil, nil)
	embeddedusage.SetAccountInspectionSnapshotHandlers(nil, nil)
}

func (s *Service) RegisterGinRoutes(group *gin.RouterGroup) {
	if s == nil || group == nil {
		return
	}
	s.RegisterAccountInspectionRoutes(group)
}

func schedulerForHandler(h *Handler) *accountInspectionScheduler {
	if h == nil {
		return nil
	}
	return h.scheduler
}
