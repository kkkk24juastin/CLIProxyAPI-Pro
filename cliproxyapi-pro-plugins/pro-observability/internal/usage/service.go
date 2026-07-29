package embeddedusage

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage/internalusage"
)

type Service struct {
	ctx              context.Context
	cfg              Config
	store            *Store
	server           *Server
	events           chan internalusage.Event
	workers          sync.WaitGroup
	done             chan struct{}
	storageMigration PluginStorageMigration
}

func Start(ctx context.Context) (*Service, error) {
	return StartWithConfig(ctx, LoadConfig())
}

func StartWithConfig(ctx context.Context, cfg Config) (*Service, error) {
	migration, err := preparePluginStorage(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("prepare plugin usage storage: %w", err)
	}
	cfg.DBPath = migration.TargetPath
	store, err := OpenStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if migration.CompletedAtMS <= 0 {
		migration.CompletedAtMS = time.Now().UnixMilli()
	}
	if err = store.completePluginStorageMigration(ctx, migration); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("complete plugin usage storage migration: %w", err)
	}

	service := &Service{
		ctx:              ctx,
		cfg:              cfg,
		store:            store,
		events:           make(chan internalusage.Event, max(cfg.BatchSize*4, 256)),
		done:             make(chan struct{}),
		storageMigration: migration,
	}
	service.server = NewServer(cfg, store)
	service.startWorker(func() { service.collect(ctx) })
	service.startWorker(func() { service.maintain(ctx) })
	service.startWorker(func() { service.runWebDAVBackups(ctx) })
	service.startWorker(func() { service.runModelPriceSync(ctx) })
	go func() {
		defer close(service.done)
		<-ctx.Done()
		service.workers.Wait()
		stopRuntimeStateWriter(service)
		if err := store.Close(); err != nil {
			log.WithError(err).Warn("failed to close embedded usage store")
		}
	}()

	log.WithFields(log.Fields{
		"db": cfg.DBPath, "migration": migration.Mode,
	}).Info("plugin usage service started")
	return service, nil
}

// Wait blocks until all background workers have flushed their pending state and
// the SQLite connection has closed. Plugin shutdown must wait before unloading
// the c-shared library.
func (s *Service) Wait() {
	if s == nil || s.done == nil {
		return
	}
	<-s.done
}

func (s *Service) StorageMigration() PluginStorageMigration {
	if s == nil {
		return PluginStorageMigration{}
	}
	return s.storageMigration
}

func (s *Service) startWorker(run func()) {
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		run()
	}()
}

func (s *Service) runModelPriceSync(ctx context.Context) {
	for {
		settings, err := s.store.GetMonitoringSettings(ctx)
		if err != nil {
			log.WithError(err).Warn("failed to load model price sync settings")
		} else if settings.ModelPriceSync.Enabled {
			state, stateErr := s.store.GetModelPriceSyncState(ctx)
			if stateErr != nil {
				log.WithError(stateErr).Warn("failed to load model price sync state")
			} else if lastRun := maxModelPriceSyncTimestamp(state.LastSuccess, state.LastAttempt); lastRun <= 0 || time.Since(time.UnixMilli(lastRun)) >= time.Duration(settings.ModelPriceSync.IntervalMinutes)*time.Minute {
				if _, syncErr := s.store.SyncModelsDevPrices(ctx, false, true); syncErr != nil {
					log.WithError(syncErr).Warn("failed to sync model prices from models.dev")
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func maxModelPriceSyncTimestamp(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (s *Service) Server() *Server {
	if s == nil {
		return nil
	}
	return s.server
}

func (s *Service) RoutingCursor(ctx context.Context, key string) (RoutingCursorState, bool, error) {
	if s == nil || s.store == nil {
		return RoutingCursorState{}, false, nil
	}
	return s.store.GetRoutingCursorState(ctx, key)
}

func (s *Service) SetRoutingCursor(ctx context.Context, cursor RoutingCursorState) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return s.store.SetRoutingCursorState(ctx, cursor)
}

func (s *Service) AuthRuntimeStats(ctx context.Context, authIndex, authID string) (AuthRuntimeStats, bool, error) {
	if s == nil || s.store == nil {
		return AuthRuntimeStats{}, false, nil
	}
	return s.store.GetAuthRuntimeStats(ctx, authIndex, authID)
}

func (s *Service) SetAuthRuntimeStats(ctx context.Context, item AuthRuntimeStats) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return s.store.SetAuthRuntimeStats(ctx, item)
}

func (s *Service) DeleteAuthRuntimeState(ctx context.Context, authID, authIndex, fileName string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return s.store.DeleteAuthRuntimeState(ctx, authID, authIndex, fileName)
}

func (s *Service) ProSetting(ctx context.Context, namespace string) (ProSetting, bool, error) {
	if s == nil || s.store == nil {
		return ProSetting{}, false, fmt.Errorf("usage service is not available")
	}
	return s.store.GetProSetting(ctx, namespace)
}

func (s *Service) SetProSetting(ctx context.Context, item ProSetting) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return s.store.SetProSetting(ctx, item)
}

// IngestRaw normalizes one plugin usage payload and queues it for durable storage.
func (s *Service) IngestRaw(ctx context.Context, raw []byte) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	event, err := internalusage.NormalizeRaw(raw)
	if err != nil {
		if addErr := s.store.AddDeadLetter(ctx, string(raw), err); addErr != nil {
			return fmt.Errorf("normalize usage event: %w; add dead letter: %v", err, addErr)
		}
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case s.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *Service) collect(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	pending := make([]internalusage.Event, 0, s.cfg.BatchSize)
	defer func() {
		if len(pending) == 0 {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.store.InsertLiveEvents(flushCtx, pending); err != nil {
			log.WithError(err).Warn("failed to flush pending embedded usage events during shutdown")
		}
	}()

	for {
		if len(pending) == 0 {
			select {
			case <-ctx.Done():
				return
			case event := <-s.events:
				pending = append(pending, event)
			case <-ticker.C:
				continue
			}
		}
		for len(pending) < s.cfg.BatchSize {
			select {
			case event := <-s.events:
				pending = append(pending, event)
			default:
				goto flush
			}
		}
	flush:
		if len(pending) == 0 {
			continue
		}
		if _, err := s.store.InsertLiveEvents(ctx, pending); err != nil {
			log.WithError(err).Warn("failed to insert embedded usage events")
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		pending = pending[:0]
	}
}

func (s *Service) maintain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextMonitoringRetentionRun(time.Now()))):
		}
		if deleted, err := s.store.ApplyRetention(ctx, time.Now()); err != nil {
			log.WithError(err).Warn("failed to apply embedded usage retention")
		} else if deleted > 0 {
			log.Infof("embedded usage retention deleted %d events", deleted)
		}
	}
}

func nextMonitoringRetentionRun(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func (s *Service) runWebDAVBackups(ctx context.Context) {
	var lastBackup time.Time
	for {
		settings, err := s.store.GetMonitoringSettings(ctx)
		if err != nil {
			log.WithError(err).Warn("failed to load monitoring settings")
		} else if shouldRunWebDAVBackup(settings, lastBackup) {
			if err := s.backupToWebDAV(ctx, settings.WebDAV); err != nil {
				log.WithError(err).Warn("failed to backup embedded usage to WebDAV")
			} else {
				lastBackup = time.Now()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func shouldRunWebDAVBackup(settings MonitoringSettings, lastBackup time.Time) bool {
	webdav := normalizeMonitoringSettings(settings).WebDAV
	if !webdav.Enabled || webdav.URL == "" {
		return false
	}
	if lastBackup.IsZero() {
		return true
	}
	return time.Since(lastBackup) >= time.Duration(webdav.IntervalMinutes)*time.Minute
}

func (s *Service) backupToWebDAV(ctx context.Context, cfg MonitoringWebDAVBackupConfig) error {
	cfg = normalizeMonitoringSettings(MonitoringSettings{WebDAV: cfg}).WebDAV
	if !cfg.Enabled || cfg.URL == "" {
		return nil
	}
	data, err := s.server.exportJSONL(ctx)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(cfg.URL, "/")
	url := baseURL + fmt.Sprintf("/usage-export-%s.jsonl", time.Now().UTC().Format("20060102_150405"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	setWebDAVAuth(req, cfg)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webdav upload failed with status %d", response.StatusCode)
	}
	log.Infof("embedded usage backup uploaded to WebDAV: %s", url)
	if cfg.RetentionDays > 0 {
		if deleted, err := pruneWebDAVBackups(ctx, baseURL, cfg, time.Now().UTC()); err != nil {
			log.WithError(err).Warn("failed to prune embedded usage WebDAV backups")
		} else if deleted > 0 {
			log.Infof("embedded usage WebDAV retention deleted %d backups", deleted)
		}
	}
	return nil
}

func setWebDAVAuth(req *http.Request, cfg MonitoringWebDAVBackupConfig) {
	if cfg.Username != "" || cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
}

type webDAVMultistatus struct {
	Responses []webDAVResponse `xml:"response"`
}

type webDAVResponse struct {
	Href     string         `xml:"href"`
	Propstat webDAVPropstat `xml:"propstat"`
}

type webDAVPropstat struct {
	Prop webDAVProp `xml:"prop"`
}

type webDAVProp struct {
	LastModified string `xml:"getlastmodified"`
}

func pruneWebDAVBackups(ctx context.Context, baseURL string, cfg MonitoringWebDAVBackupConfig, now time.Time) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", baseURL+"/", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Depth", "1")
	setWebDAVAuth(req, cfg)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("webdav propfind failed with status %d", response.StatusCode)
	}
	var listing webDAVMultistatus
	if err := xml.NewDecoder(response.Body).Decode(&listing); err != nil {
		return 0, err
	}

	cutoff := now.AddDate(0, 0, -cfg.RetentionDays)
	deleted := 0
	for _, item := range listing.Responses {
		fileName := path.Base(strings.TrimRight(item.Href, "/"))
		if !strings.HasPrefix(fileName, "usage-export-") || !strings.HasSuffix(fileName, ".jsonl") {
			continue
		}
		modifiedAt, err := http.ParseTime(strings.TrimSpace(item.Propstat.Prop.LastModified))
		if err != nil || !modifiedAt.Before(cutoff) {
			continue
		}
		deleteReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/"+fileName, nil)
		if err != nil {
			return deleted, err
		}
		setWebDAVAuth(deleteReq, cfg)
		deleteResponse, err := http.DefaultClient.Do(deleteReq)
		if err != nil {
			return deleted, err
		}
		deleteResponse.Body.Close()
		if deleteResponse.StatusCode < 200 || deleteResponse.StatusCode >= 300 {
			return deleted, fmt.Errorf("webdav delete failed for %s with status %d", fileName, deleteResponse.StatusCode)
		}
		deleted++
	}
	return deleted, nil
}
