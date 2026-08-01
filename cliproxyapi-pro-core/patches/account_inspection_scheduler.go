package management

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/misc"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
	probackup "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/backup"
	proinspection "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/inspection"
	proquota "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/quota"
	log "github.com/sirupsen/logrus"
)

const (
	accountInspectionProviderAll            = proinspection.ProviderAll
	accountInspectionDefaultIntervalMin     = proinspection.DefaultIntervalMin
	accountInspectionDefaultTimeoutMS       = proinspection.DefaultTimeoutMS
	accountInspectionMinTimeoutMS           = proinspection.MinTimeoutMS
	accountInspectionMaxTimeoutMS           = proinspection.MaxTimeoutMS
	accountInspectionMaxWorkers             = proinspection.MaxWorkers
	accountInspectionMaxDeleteWorkers       = proinspection.MaxDeleteWorkers
	accountInspectionMaxRetries             = proinspection.MaxRetries
	accountInspectionMaxRunDuration         = 30 * time.Minute
	accountInspectionMaxProviderConcurrency = 2
	accountInspectionMaxRefreshConcurrency  = 2
	accountInspectionXAIRetryDelay          = 300 * time.Millisecond
	accountInspectionWebSocketWriteTimeout  = 5 * time.Second
	accountInspectionWebSocketPongWait      = 60 * time.Second
	accountInspectionWebSocketPingPeriod    = 54 * time.Second
	accountInspectionProgressBroadcastGap   = 500 * time.Millisecond
	accountInspectionMaxResultPageSize      = 500
	accountInspectionMaxLogPageSize         = 500
	accountInspectionQuotaParserVersion     = proquota.CacheParserVersion
)

var accountInspectionWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var accountInspectionSupportedProviders = proinspection.SupportedProviderSet()

var accountInspectionSchedulers sync.Map

type accountInspectionSettings = proinspection.Settings
type accountInspectionSchedule = proinspection.Schedule

type accountInspectionLogEntry = proinspection.LogEntry

type accountInspectionResult = proinspection.Result
type accountInspectionSummary = proinspection.Summary
type accountInspectionHealthCounts = proinspection.HealthCounts

type accountInspectionRunState = proinspection.RunState

type accountInspectionStreamMessageType = proinspection.StreamMessageType

type accountInspectionDeepProbeStatus = proinspection.DeepProbeStatus

type accountInspectionAntigravityQuotaMode = proinspection.AntigravityQuotaMode

type accountInspectionAction = proinspection.Action

const (
	accountInspectionStreamSnapshot = proinspection.StreamSnapshot
	accountInspectionStreamLog      = proinspection.StreamLog
	accountInspectionStreamStatus   = proinspection.StreamStatus
)

const (
	accountInspectionActionNone    = proinspection.ActionNone
	accountInspectionActionKeep    = proinspection.ActionKeep
	accountInspectionActionDelete  = proinspection.ActionDelete
	accountInspectionActionDisable = proinspection.ActionDisable
	accountInspectionActionEnable  = proinspection.ActionEnable
)

const (
	accountInspectionDeepProbeSuccess        = proinspection.DeepProbeSuccess
	accountInspectionDeepProbeQuota          = proinspection.DeepProbeQuota
	accountInspectionDeepProbeAuthError      = proinspection.DeepProbeAuthError
	accountInspectionDeepProbeTransientError = proinspection.DeepProbeTransientError
	accountInspectionDeepProbeSkipped        = proinspection.DeepProbeSkipped
)

const (
	accountInspectionAntigravityQuotaModeMaxUsed   = proinspection.AntigravityQuotaModeMaxUsed
	accountInspectionAntigravityQuotaModeClaudeGpt = proinspection.AntigravityQuotaModeClaudeGPT
)

const antigravityCodeAssistURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"

const (
	accountInspectionStateIdle      = proinspection.RunStateIdle
	accountInspectionStateRunning   = proinspection.RunStateRunning
	accountInspectionStatePaused    = proinspection.RunStatePaused
	accountInspectionStateStopping  = proinspection.RunStateStopping
	accountInspectionStateStopped   = proinspection.RunStateStopped
	accountInspectionStateCompleted = proinspection.RunStateCompleted
	accountInspectionStatePartial   = proinspection.RunStatePartial
	accountInspectionStateFailed    = proinspection.RunStateFailed
)

const accountInspectionResultSnapshotVersion = proinspection.ResultSnapshotVersion

var errAccountInspectionRestoredSnapshotReadOnly = errors.New("restored account inspection snapshot is read-only; run a new inspection first")
var errAccountInspectionResultStale = errors.New("account inspection result is stale or no longer available")
var errAccountInspectionSharedSourceDelete = errors.New("cannot delete one plugin virtual auth from a shared source file")

type accountInspectionProgress = proinspection.Progress
type accountInspectionStatus = proinspection.Status

type accountInspectionResultSnapshot = proinspection.ResultSnapshot

type accountInspectionPageInfo = proinspection.PageInfo

type accountInspectionSnapshotOptions = proinspection.SnapshotOptions

type accountInspectionHealthBucket = proinspection.HealthBucket

const (
	accountInspectionHealthHealthy         = proinspection.HealthHealthy
	accountInspectionHealthDisabled        = proinspection.HealthDisabled
	accountInspectionHealthAuthInvalid     = proinspection.HealthAuthInvalid
	accountInspectionHealthQuotaExhausted  = proinspection.HealthQuotaExhausted
	accountInspectionHealthInspectionError = proinspection.HealthInspectionError
	accountInspectionHealthRecoverable     = proinspection.HealthRecoverable
)

type accountInspectionLogStreamMessage = proinspection.LogStreamMessage

type accountInspectionScheduler struct {
	h                       *Handler
	quota                   proinspection.QuotaGateway
	path                    string
	snapshotPath            string
	trigger                 chan struct{}
	mu                      sync.Mutex
	actionMu                sync.Mutex
	pause                   *sync.Cond
	cancel                  context.CancelFunc
	schedule                accountInspectionSchedule
	lastRunSettings         accountInspectionSettings
	status                  accountInspectionStatus
	healthCounts            accountInspectionHealthCounts
	autoActionConfirmations *proinspection.ConfirmationCounter
	subscribers             map[chan accountInspectionLogStreamMessage]struct{}
	lastProgressBroadcastAt int64
	xaiDeepProbeOnce        sync.Once
	xaiDeepProbeGate        chan struct{}
	runWG                   sync.WaitGroup
	stopped                 bool
	lifecycle               *proinspection.Lifecycle
	backupUnregister        func()
	backupHookUnregister    func()
}

type accountInspectionAccount struct {
	Auth        *coreauth.Auth
	Key         string
	Provider    string
	FileName    string
	DisplayName string
	Email       string
	Name        string
	AuthIndex   string
	Disabled    bool
}

type accountInspectionHTTPResult = proinspection.ProbeResponse

type accountInspectionDecision = proinspection.Decision

type accountInspectionActionItem = proinspection.ActionItem
type accountInspectionActionRequest = proinspection.ActionRequest
type accountInspectionOneRequest = proinspection.OneRequest
type accountInspectionRefreshTokenRequest = proinspection.RefreshTokenRequest
type accountInspectionActionOutcome = proinspection.ActionOutcome

func (h *Handler) startAccountInspectionScheduler(quota proinspection.QuotaGateway) {
	if h == nil {
		return
	}
	if _, loaded := accountInspectionSchedulers.LoadOrStore(h, newAccountInspectionScheduler(h, quota)); loaded {
		return
	}
	scheduler := schedulerForHandler(h)
	if scheduler != nil {
		unregisterHooks := []func(){
			embeddedusage.RegisterAccountInspectionScheduleHandlers(scheduler.exportSchedule, scheduler.importSchedule),
			embeddedusage.RegisterAccountInspectionSnapshotHandlers(scheduler.exportResultSnapshot, scheduler.importResultSnapshot),
			embeddedusage.RegisterLegacyQuotaCleanupHandler(func(ctx context.Context) error {
				scheduler.cleanupLegacyQuotaCaches(ctx)
				return nil
			}),
		}
		if h.authManager != nil {
			unregisterHooks = append(unregisterHooks, embeddedusage.RegisterAuthRuntimeStateImportHandler(h.authManager.ApplyImportedRuntimeState))
		}
		scheduler.backupHookUnregister = func() {
			for index := len(unregisterHooks) - 1; index >= 0; index-- {
				unregisterHooks[index]()
			}
		}
		scheduler.backupUnregister = probackup.Default.RegisterLifecycle(probackup.Lifecycle{
			Pause:  scheduler.pauseForBackup,
			Resume: scheduler.lifecycle.Resume,
		})
		scheduler.cleanupLegacyQuotaCaches(context.Background())
		if h.lifecycleContext != nil {
			h.lifecycleWG.Add(1)
			go func() {
				defer h.lifecycleWG.Done()
				scheduler.loop(h.lifecycleContext)
			}()
		}
	}
}

func schedulerForHandler(h *Handler) *accountInspectionScheduler {
	if h == nil {
		return nil
	}
	value, ok := accountInspectionSchedulers.Load(h)
	if !ok {
		return nil
	}
	scheduler, _ := value.(*accountInspectionScheduler)
	return scheduler
}

func newAccountInspectionScheduler(h *Handler, quota proinspection.QuotaGateway) *accountInspectionScheduler {
	schedulePath := accountInspectionSchedulePath()
	scheduler := &accountInspectionScheduler{
		h:                       h,
		quota:                   quota,
		path:                    schedulePath,
		snapshotPath:            accountInspectionResultSnapshotPath(schedulePath),
		trigger:                 make(chan struct{}, 1),
		subscribers:             make(map[chan accountInspectionLogStreamMessage]struct{}),
		autoActionConfirmations: proinspection.NewConfirmationCounter(),
		lifecycle:               &proinspection.Lifecycle{},
		schedule: accountInspectionSchedule{
			Enabled:         false,
			IntervalMinutes: accountInspectionDefaultIntervalMin,
			Settings:        defaultAccountInspectionSettings(),
		},
		status: accountInspectionStatus{State: accountInspectionStateIdle},
	}
	scheduler.lastRunSettings = scheduler.schedule.Settings
	scheduler.pause = sync.NewCond(&scheduler.mu)
	scheduler.load()
	return scheduler
}

func accountInspectionSchedulePath() string {
	if value := strings.TrimSpace(os.Getenv("ACCOUNT_INSPECTION_SCHEDULE_PATH")); value != "" {
		return value
	}
	dataDir := strings.TrimSpace(os.Getenv("USAGE_DATA_DIR"))
	if dataDir == "" {
		dataDir = "/CLIProxyAPI/usage"
	}
	return filepath.Join(dataDir, "account-inspection-schedule.json")
}

func accountInspectionResultSnapshotPath(schedulePath string) string {
	if value := strings.TrimSpace(os.Getenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH")); value != "" {
		return value
	}
	return filepath.Join(filepath.Dir(schedulePath), "account-inspection-snapshot.json")
}

func defaultAccountInspectionSettings() accountInspectionSettings {
	return proinspection.DefaultSettings()
}

func normalizeAccountInspectionSchedule(input accountInspectionSchedule) accountInspectionSchedule {
	return proinspection.NormalizeSchedule(input, time.Now())
}

func normalizeAccountInspectionAutoAction(action accountInspectionAction) accountInspectionAction {
	return proinspection.NormalizeAutoAction(action)
}

func (s *accountInspectionScheduler) load() {
	raw, err := os.ReadFile(s.path)
	if err == nil {
		var schedule accountInspectionSchedule
		if err := json.Unmarshal(raw, &schedule); err != nil {
			log.WithError(err).Warn("failed to load account inspection schedule")
		} else {
			s.schedule = normalizeAccountInspectionSchedule(schedule)
		}
	}
	if err := s.loadResultSnapshot(); err != nil {
		log.WithError(err).Warn("failed to load account inspection snapshot")
	}
}

func (s *accountInspectionScheduler) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.schedule, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o600)
}

func normalizeAccountInspectionSnapshotState(state accountInspectionRunState) accountInspectionRunState {
	return proinspection.NormalizeSnapshotState(state)
}

func decodeAccountInspectionResultSnapshot(raw []byte) (accountInspectionResultSnapshot, error) {
	return proinspection.DecodeResultSnapshot(raw, time.Now())
}

func (s *accountInspectionScheduler) resultSnapshotLocked() (accountInspectionResultSnapshot, bool) {
	if s == nil || s.status.LastFinishedAt <= 0 {
		return accountInspectionResultSnapshot{}, false
	}
	settings := s.lastRunSettings
	if strings.TrimSpace(settings.TargetType) == "" {
		settings = s.schedule.Settings
	}
	return accountInspectionResultSnapshot{
		Version:        accountInspectionResultSnapshotVersion,
		State:          normalizeAccountInspectionSnapshotState(s.status.State),
		LastStartedAt:  s.status.LastStartedAt,
		LastFinishedAt: s.status.LastFinishedAt,
		LastError:      s.status.LastError,
		Settings:       settings,
		Summary:        s.status.Summary,
		HealthCounts:   s.healthCountsLocked(),
		Results:        append([]accountInspectionResult(nil), s.status.Results...),
	}, true
}

func (s *accountInspectionScheduler) applyResultSnapshotLocked(snapshot accountInspectionResultSnapshot, restored bool) {
	s.lastRunSettings = snapshot.Settings
	s.status.State = snapshot.State
	s.status.LastStartedAt = snapshot.LastStartedAt
	s.status.LastFinishedAt = snapshot.LastFinishedAt
	s.status.LastError = snapshot.LastError
	s.status.Progress = accountInspectionProgress{
		Total:     len(snapshot.Results),
		Completed: len(snapshot.Results),
	}
	s.status.Summary = snapshot.Summary
	s.status.Logs = nil
	s.status.Results = append([]accountInspectionResult(nil), snapshot.Results...)
	s.status.RestoredSnapshot = restored
	s.healthCounts = snapshot.HealthCounts
}

func (s *accountInspectionScheduler) saveResultSnapshotLocked() error {
	snapshot, ok := s.resultSnapshotLocked()
	if !ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.snapshotPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.snapshotPath, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(s.snapshotPath, 0o600)
}

func (s *accountInspectionScheduler) loadResultSnapshot() error {
	if s == nil {
		return nil
	}
	raw, err := os.ReadFile(s.snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil {
		return err
	}
	s.applyResultSnapshotLocked(snapshot, true)
	return nil
}

func (s *accountInspectionScheduler) exportResultSnapshot() ([]byte, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	snapshot, ok := s.resultSnapshotLocked()
	s.mu.Unlock()
	if !ok {
		raw, err := os.ReadFile(s.snapshotPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		snapshot, err = decodeAccountInspectionResultSnapshot(raw)
		if err != nil {
			return nil, false, err
		}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *accountInspectionScheduler) importResultSnapshot(raw []byte) error {
	if s == nil {
		return nil
	}
	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.isRunningLocked() {
		s.mu.Unlock()
		return fmt.Errorf("account inspection is running")
	}
	s.applyResultSnapshotLocked(snapshot, true)
	err = s.saveResultSnapshotLocked()
	broadcast := s.statusBroadcastLocked()
	s.mu.Unlock()
	broadcast.send()
	return err
}

func (s *accountInspectionScheduler) snapshotWithOptions(options accountInspectionSnapshotOptions) gin.H {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gin.H{
		"schedule": s.schedule,
		"status":   s.streamStatusLocked(options),
	}
}

func accountInspectionRequestSnapshotOptions(c *gin.Context) accountInspectionSnapshotOptions {
	value := strings.ToLower(strings.TrimSpace(c.Query("details")))
	resultPageSize := parseAccountInspectionQueryInt(c, "result_page_size", 100)
	if strings.TrimSpace(c.Query("result_page_size")) == "" {
		resultPageSize = parseAccountInspectionQueryInt(c, "result_limit", resultPageSize)
	}
	logPageSize := parseAccountInspectionQueryInt(c, "log_page_size", 100)
	if strings.TrimSpace(c.Query("log_page_size")) == "" {
		logPageSize = parseAccountInspectionQueryInt(c, "log_limit", logPageSize)
	}
	return accountInspectionSnapshotOptions{
		IncludeDetails:    value != "0" && value != "false" && value != "summary",
		ResultPage:        parseAccountInspectionQueryInt(c, "result_page", 1),
		ResultPageSize:    resultPageSize,
		ResultFilter:      strings.ToLower(strings.TrimSpace(c.Query("result_filter"))),
		ResultPendingOnly: parseAccountInspectionQueryBool(c, "result_pending_only"),
		ResultProvider:    strings.ToLower(strings.TrimSpace(c.Query("result_provider"))),
		ResultSearch:      strings.TrimSpace(c.Query("result_search")),
		LogPage:           parseAccountInspectionQueryInt(c, "log_page", 1),
		LogPageSize:       logPageSize,
		LogLevel:          strings.ToLower(strings.TrimSpace(c.Query("log_level"))),
	}
}

func parseAccountInspectionQueryBool(c *gin.Context, key string) bool {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func parseAccountInspectionQueryInt(c *gin.Context, key string, fallback int) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func (s *accountInspectionScheduler) snapshotForRequest(c *gin.Context) gin.H {
	return s.snapshotWithOptions(accountInspectionRequestSnapshotOptions(c))
}

func accountInspectionResultHealthCounts(results []accountInspectionResult) accountInspectionHealthCounts {
	return proinspection.ResultHealthCounts(results)
}

func accountInspectionResultProviderHealthCounts(results []accountInspectionResult) map[string]accountInspectionHealthCounts {
	return proinspection.ResultProviderHealthCounts(results)
}

func adjustAccountInspectionHealthCountsForResult(counts accountInspectionHealthCounts, result accountInspectionResult, delta int) accountInspectionHealthCounts {
	return proinspection.AdjustHealthCountsForResult(counts, result, delta)
}

func (s *accountInspectionScheduler) healthCountsLocked() accountInspectionHealthCounts {
	if s.healthCounts.Total == len(s.status.Results) {
		return s.healthCounts
	}
	s.healthCounts = accountInspectionResultHealthCounts(s.status.Results)
	return s.healthCounts
}

func accountInspectionResultHealthBucketOf(result accountInspectionResult) accountInspectionHealthBucket {
	return proinspection.HealthBucketOf(result)
}

func isAccountInspectionQuotaResult(result accountInspectionResult) bool {
	return proinspection.IsQuotaResult(result)
}

func normalizeAccountInspectionResultSemantics(result accountInspectionResult) accountInspectionResult {
	return proinspection.NormalizeResultSemantics(result)
}

func accountInspectionResultMatchesFilter(result accountInspectionResult, filter string) bool {
	return proinspection.ResultMatchesFilter(result, filter)
}

func accountInspectionResultMatchesProvider(result accountInspectionResult, provider string) bool {
	return proinspection.ResultMatchesProvider(result, provider)
}

func accountInspectionResultMatchesSearch(result accountInspectionResult, search string) bool {
	return proinspection.ResultMatchesSearch(result, search)
}

func paginateAccountInspectionLogs(logs []accountInspectionLogEntry, page int, pageSize int, level string) ([]accountInspectionLogEntry, accountInspectionPageInfo) {
	return proinspection.PaginateLogs(logs, page, pageSize, accountInspectionMaxLogPageSize, level)
}

func paginateAccountInspectionResults(results []accountInspectionResult, page int, pageSize int, filter string, pendingOnly bool, provider string, search string) ([]accountInspectionResult, accountInspectionPageInfo) {
	return proinspection.PaginateResults(results, page, pageSize, accountInspectionMaxResultPageSize, filter, pendingOnly, provider, search)
}

func (s *accountInspectionScheduler) streamStatusLocked(options accountInspectionSnapshotOptions) accountInspectionStatus {
	healthCounts := s.healthCounts
	if options.IncludeDetails {
		healthCounts = s.healthCountsLocked()
	}
	return proinspection.ProjectStatus(s.status, healthCounts, options, accountInspectionMaxResultPageSize, accountInspectionMaxLogPageSize)
}

func (s *accountInspectionScheduler) streamMessageLocked(messageType accountInspectionStreamMessageType, options accountInspectionSnapshotOptions, logEntry *accountInspectionLogEntry) accountInspectionLogStreamMessage {
	return accountInspectionLogStreamMessage{Type: messageType, Schedule: s.schedule, Status: s.streamStatusLocked(options), Log: logEntry}
}

func (s *accountInspectionScheduler) snapshotStreamMessageLocked(options accountInspectionSnapshotOptions) accountInspectionLogStreamMessage {
	return s.streamMessageLocked(accountInspectionStreamSnapshot, options, nil)
}

func (s *accountInspectionScheduler) statusStreamMessageLocked(includeDetails bool) accountInspectionLogStreamMessage {
	return s.streamMessageLocked(accountInspectionStreamStatus, accountInspectionSnapshotOptions{IncludeDetails: includeDetails}, nil)
}

func (s *accountInspectionScheduler) logStreamMessageLocked(entry accountInspectionLogEntry) accountInspectionLogStreamMessage {
	return s.streamMessageLocked(accountInspectionStreamLog, accountInspectionSnapshotOptions{}, &entry)
}

type accountInspectionBroadcast struct {
	subscribers []chan accountInspectionLogStreamMessage
	message     accountInspectionLogStreamMessage
}

func (broadcast accountInspectionBroadcast) send() {
	for _, subscriber := range broadcast.subscribers {
		select {
		case subscriber <- broadcast.message:
		default:
		}
	}
}

func (s *accountInspectionScheduler) subscribersLocked() []chan accountInspectionLogStreamMessage {
	subscribers := make([]chan accountInspectionLogStreamMessage, 0, len(s.subscribers))
	for subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	return subscribers
}

func (s *accountInspectionScheduler) statusBroadcastLocked() accountInspectionBroadcast {
	return accountInspectionBroadcast{
		subscribers: s.subscribersLocked(),
		message:     s.statusStreamMessageLocked(false),
	}
}

func (s *accountInspectionScheduler) logBroadcastLocked(entry accountInspectionLogEntry) accountInspectionBroadcast {
	return accountInspectionBroadcast{
		subscribers: s.subscribersLocked(),
		message:     s.logStreamMessageLocked(entry),
	}
}

func (s *accountInspectionScheduler) subscribeLogs(options accountInspectionSnapshotOptions) (chan accountInspectionLogStreamMessage, accountInspectionLogStreamMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subscriber := make(chan accountInspectionLogStreamMessage, 16)
	s.subscribers[subscriber] = struct{}{}
	return subscriber, s.snapshotStreamMessageLocked(options)
}

func (s *accountInspectionScheduler) unsubscribeLogs(subscriber chan accountInspectionLogStreamMessage) {
	s.mu.Lock()
	delete(s.subscribers, subscriber)
	s.mu.Unlock()
}

func (s *accountInspectionScheduler) isRunningLocked() bool {
	return s.status.State == accountInspectionStateRunning || s.status.State == accountInspectionStatePaused || s.status.State == accountInspectionStateStopping
}

func (s *accountInspectionScheduler) isPausedLocked() bool {
	return s.status.State == accountInspectionStatePaused
}

func (s *accountInspectionScheduler) isStoppingLocked() bool {
	return s.status.State == accountInspectionStateStopping
}

func (s *accountInspectionScheduler) setRunStateLocked(state accountInspectionRunState) {
	s.status.State = state
}

func (s *accountInspectionScheduler) exportSchedule() ([]byte, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	schedule := s.schedule
	s.mu.Unlock()
	raw, err := json.Marshal(schedule)
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *accountInspectionScheduler) importSchedule(raw []byte) error {
	if s == nil {
		return nil
	}
	var schedule accountInspectionSchedule
	if err := json.Unmarshal(raw, &schedule); err != nil {
		return err
	}
	schedule.NextRunAt = 0
	return s.update(schedule)
}

func (s *accountInspectionScheduler) update(schedule accountInspectionSchedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return fmt.Errorf("account inspection scheduler is shut down")
	}
	previousNextRunAt := s.schedule.NextRunAt
	s.schedule = normalizeAccountInspectionSchedule(schedule)
	if s.schedule.Enabled && previousNextRunAt > 0 && schedule.NextRunAt == 0 {
		s.schedule.NextRunAt = previousNextRunAt
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
	return nil
}

func (s *accountInspectionScheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.maybeRunDue()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.trigger:
		}
	}
}

func (s *accountInspectionScheduler) maybeRunDue() {
	s.mu.Lock()
	schedule := s.schedule
	running := s.isRunningLocked()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || !schedule.Enabled || running || schedule.NextRunAt <= 0 || time.Now().UnixMilli() < schedule.NextRunAt {
		return
	}
	go func() { _ = s.startRun(false) }()
}

func (s *accountInspectionScheduler) beginLifecycle() (func(), error) {
	if s == nil || s.lifecycle == nil {
		return func() {}, nil
	}
	return s.lifecycle.Begin()
}

func (s *accountInspectionScheduler) pauseForBackup(ctx context.Context) error {
	if s == nil || s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.PauseAndCancel(ctx, s.stopRun)
}

func (s *accountInspectionScheduler) startRun(manual bool) error {
	release, err := s.beginLifecycle()
	if err != nil {
		return err
	}
	baseContext := context.Background()
	if s != nil && s.h != nil && s.h.lifecycleContext != nil {
		baseContext = s.h.lifecycleContext
	}
	ctx, cancel := context.WithTimeout(baseContext, accountInspectionMaxRunDuration)
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		cancel()
		release()
		return fmt.Errorf("account inspection scheduler is shut down")
	}
	if s.isRunningLocked() {
		s.mu.Unlock()
		cancel()
		release()
		return fmt.Errorf("account inspection already running")
	}
	s.cancel = cancel
	s.lastRunSettings = s.schedule.Settings
	s.setRunStateLocked(accountInspectionStateRunning)
	s.status.RestoredSnapshot = false
	s.status.LastStartedAt = time.Now().UnixMilli()
	s.status.LastFinishedAt = 0
	s.status.LastError = ""
	s.status.Progress = accountInspectionProgress{}
	s.status.Summary = accountInspectionSummary{}
	s.status.Logs = nil
	s.status.Results = nil
	s.healthCounts = accountInspectionHealthCounts{}
	schedule := s.schedule
	s.runWG.Add(2)
	s.mu.Unlock()

	go func() {
		defer s.runWG.Done()
		<-ctx.Done()
		s.mu.Lock()
		s.pause.Broadcast()
		s.mu.Unlock()
	}()
	go func() {
		defer s.runWG.Done()
		defer release()
		s.run(ctx, cancel, schedule, manual)
	}()
	return nil
}

func (s *accountInspectionScheduler) shutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	s.pause.Broadcast()
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.runWG.Wait()
}

func (s *accountInspectionScheduler) appendLog(level string, message string) {
	entry := accountInspectionLogEntry{Time: time.Now().UnixMilli(), Level: level, Message: message}
	s.mu.Lock()
	s.status.Logs = append(s.status.Logs, entry)
	if len(s.status.Logs) > 200 {
		s.status.Logs = s.status.Logs[len(s.status.Logs)-200:]
	}
	broadcast := s.logBroadcastLocked(entry)
	s.mu.Unlock()
	broadcast.send()
}

func (s *accountInspectionScheduler) updateProgress(total int, completed int, inFlight int, force bool) {
	pending := total - completed - inFlight
	if pending < 0 {
		pending = 0
	}
	now := time.Now().UnixMilli()
	s.mu.Lock()
	previous := s.status.Progress
	next := accountInspectionProgress{Total: total, Completed: completed, InFlight: inFlight, Pending: pending}
	if previous == next {
		s.mu.Unlock()
		return
	}
	s.status.Progress = next
	shouldBroadcast := force || completed == total || now-s.lastProgressBroadcastAt >= accountInspectionProgressBroadcastGap.Milliseconds()
	var broadcast accountInspectionBroadcast
	if shouldBroadcast {
		s.lastProgressBroadcastAt = now
		broadcast = s.statusBroadcastLocked()
	}
	s.mu.Unlock()
	broadcast.send()
}

func (s *accountInspectionScheduler) waitIfPaused(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.isPausedLocked() && !s.isStoppingLocked() {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.pause.Wait()
	}
	return ctx.Err()
}

func (s *accountInspectionScheduler) pauseRun() {
	var broadcast accountInspectionBroadcast
	s.mu.Lock()
	if s.isRunningLocked() && !s.isStoppingLocked() {
		s.setRunStateLocked(accountInspectionStatePaused)
		broadcast = s.statusBroadcastLocked()
	}
	s.mu.Unlock()
	broadcast.send()
}

func (s *accountInspectionScheduler) resumeRun() {
	var broadcast accountInspectionBroadcast
	s.mu.Lock()
	if s.isRunningLocked() && s.isPausedLocked() {
		s.setRunStateLocked(accountInspectionStateRunning)
		broadcast = s.statusBroadcastLocked()
		s.pause.Broadcast()
	}
	s.mu.Unlock()
	broadcast.send()
}

func (s *accountInspectionScheduler) stopRun() {
	var broadcast accountInspectionBroadcast
	s.mu.Lock()
	cancel := s.cancel
	if s.isRunningLocked() {
		s.setRunStateLocked(accountInspectionStateStopping)
		broadcast = s.statusBroadcastLocked()
		s.pause.Broadcast()
	}
	s.mu.Unlock()
	broadcast.send()
	if cancel != nil {
		cancel()
	}
}

func (s *accountInspectionScheduler) inspectOne(ctx context.Context, item accountInspectionActionItem) (accountInspectionResult, error) {
	release, err := s.beginLifecycle()
	if err != nil {
		return accountInspectionResult{}, err
	}
	defer release()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, accountInspectionMaxRunDuration)
	defer cancel()
	s.mu.Lock()
	if s.status.RestoredSnapshot {
		s.mu.Unlock()
		return accountInspectionResult{}, errAccountInspectionRestoredSnapshotReadOnly
	}
	if s.isRunningLocked() {
		s.mu.Unlock()
		return accountInspectionResult{}, fmt.Errorf("account inspection already running")
	}
	schedule := s.schedule
	s.mu.Unlock()
	boundItem, err := s.bindActionItemToSnapshot(item)
	if err != nil {
		return accountInspectionResult{}, err
	}

	result, _, runErr := s.executeSingleInspection(ctx, schedule.Settings, boundItem)
	if runErr != nil {
		s.appendLog("error", fmt.Sprintf("重新检查失败：%s", runErr.Error()))
		return result, runErr
	}

	s.mu.Lock()
	var saveErr error
	if !s.isRunningLocked() {
		s.mergeSingleInspectionResultLocked(result)
		s.status.Results = sortAccountInspectionResults(s.status.Results)
		saveErr = s.saveResultSnapshotLocked()
	}
	broadcast := s.statusBroadcastLocked()
	s.mu.Unlock()
	broadcast.send()
	if saveErr != nil {
		return result, fmt.Errorf("failed to save account inspection snapshot: %w", saveErr)
	}
	return result, nil
}

func (s *accountInspectionScheduler) refreshTokenNow(ctx context.Context, item accountInspectionActionItem) (accountInspectionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return accountInspectionResult{}, fmt.Errorf("account inspection scheduler unavailable")
	}
	s.mu.Lock()
	restoredSnapshot := s.status.RestoredSnapshot
	s.mu.Unlock()
	if restoredSnapshot {
		return accountInspectionResult{}, errAccountInspectionRestoredSnapshotReadOnly
	}
	if s.h == nil || s.h.authManager == nil {
		return accountInspectionResult{}, fmt.Errorf("core auth manager unavailable")
	}
	boundItem, err := s.bindActionItemToSnapshot(item)
	if err != nil {
		return accountInspectionResult{}, err
	}
	auths, err := s.auths()
	if err != nil {
		return accountInspectionResult{}, err
	}
	for _, auth := range auths {
		account := accountFromAuth(auth)
		if boundItem.Key != "" && account.Key != boundItem.Key {
			continue
		}
		if boundItem.Key == "" && (account.FileName != boundItem.FileName || account.AuthIndex != boundItem.AuthIndex) {
			continue
		}
		result := account.baseResult()
		result.TokenRefreshTriggered = true
		if account.Auth == nil || account.Auth.ID == "" {
			result.TokenRefreshStatus = "failed"
			result.TokenRefreshError = "missing auth id"
			result.Error = result.TokenRefreshError
			result.ErrorCode = "missing_auth_id"
			result.ActionReason = "刷新令牌失败，保留账号"
			s.syncInspectionAuthError(ctx, account, "token_refresh_error", result.TokenRefreshError, 0)
			return result, errors.New(result.TokenRefreshError)
		}
		s.appendLog("info", fmt.Sprintf("主动刷新令牌 %s", account.identity()))
		updated, refreshed, refreshErr := s.h.authManager.ForceRefreshForInspection(ctx, account.Auth.ID)
		if updated != nil {
			account = accountFromAuth(updated)
			result = account.baseResult()
		}
		result.TokenRefreshTriggered = true
		result.NextRefreshAt = account.nextRefreshAtMillis()
		if refreshErr != nil {
			result.TokenRefreshStatus = "failed"
			result.TokenRefreshError = refreshErr.Error()
			result.Error = refreshErr.Error()
			result.ErrorCode = "token_refresh_error"
			result.ActionReason = "刷新令牌失败，保留账号"
			s.syncInspectionAuthError(ctx, account, "token_refresh_error", refreshErr.Error(), 0)
			s.appendLog("warning", fmt.Sprintf("%s 主动刷新令牌失败：%s", account.identity(), refreshErr.Error()))
			return result, refreshErr
		}
		if refreshed {
			result.TokenRefreshStatus = "success"
			s.appendLog("success", fmt.Sprintf("%s 主动刷新令牌成功", account.identity()))
		} else {
			result.TokenRefreshStatus = ""
			s.appendLog("warning", fmt.Sprintf("%s 主动刷新令牌未执行", account.identity()))
		}
		return result, nil
	}
	return accountInspectionResult{}, fmt.Errorf("account not found")
}

func sameAccountInspectionResult(a accountInspectionResult, b accountInspectionResult) bool {
	return proinspection.SameResult(a, b)
}

func (s *accountInspectionScheduler) updateInspectionResultLocked(result accountInspectionResult, appendMissing bool, update func(accountInspectionResult) (accountInspectionResult, bool)) bool {
	if result.Key == "" {
		return false
	}

	for index, current := range s.status.Results {
		if sameAccountInspectionResult(current, result) {
			merged, updateSummary := update(current)
			if updateSummary {
				s.status.Summary = adjustAccountInspectionSummaryForResult(s.status.Summary, current, -1)
				s.status.Summary = adjustAccountInspectionSummaryForResult(s.status.Summary, merged, 1)
			}
			s.healthCounts = adjustAccountInspectionHealthCountsForResult(s.healthCounts, current, -1)
			s.healthCounts = adjustAccountInspectionHealthCountsForResult(s.healthCounts, merged, 1)
			s.status.Results[index] = merged
			return true
		}
	}

	if !appendMissing {
		return false
	}
	s.status.Summary = adjustAccountInspectionSummaryForResult(s.status.Summary, result, 1)
	s.healthCounts = adjustAccountInspectionHealthCountsForResult(s.healthCounts, result, 1)
	s.status.Results = append(s.status.Results, result)
	return true
}

func (s *accountInspectionScheduler) mergeTokenRefreshResultLocked(result accountInspectionResult) {
	s.updateInspectionResultLocked(result, true, func(current accountInspectionResult) (accountInspectionResult, bool) {
		return proinspection.MergeTokenRefreshResult(current, result)
	})
}

func (s *accountInspectionScheduler) mergeSingleInspectionResultLocked(result accountInspectionResult) {
	s.updateInspectionResultLocked(result, false, func(current accountInspectionResult) (accountInspectionResult, bool) {
		return proinspection.MergeReinspectionResult(current, result)
	})
}

func (s *accountInspectionScheduler) executeSingleInspection(ctx context.Context, settings accountInspectionSettings, item accountInspectionActionItem) (accountInspectionResult, accountInspectionSummary, error) {
	auths, err := s.auths()
	if err != nil {
		return accountInspectionResult{}, accountInspectionSummary{}, err
	}
	for _, auth := range auths {
		account := accountFromAuth(auth)
		if item.Key != "" && account.Key != item.Key {
			continue
		}
		if item.Key == "" && (account.FileName != item.FileName || account.AuthIndex != item.AuthIndex) {
			continue
		}
		if !shouldInspectAccount(account, accountInspectionProviderAll) {
			return accountInspectionResult{}, accountInspectionSummary{}, fmt.Errorf("unsupported provider")
		}
		s.appendLog("info", fmt.Sprintf("重新检查 %s", account.identity()))
		result := s.inspectAccount(ctx, account, settings, make(chan struct{}, accountInspectionMaxRefreshConcurrency))
		return result, summarizeAccountInspection(len(auths), 1, []accountInspectionAccount{account}, []accountInspectionResult{result}), nil
	}
	return accountInspectionResult{}, accountInspectionSummary{}, fmt.Errorf("account not found")
}

func (s *accountInspectionScheduler) run(ctx context.Context, cancel context.CancelFunc, schedule accountInspectionSchedule, manual bool) {
	defer cancel()
	s.appendLog("info", "后端账号巡检开始")
	results, summary, runErr := s.executeInspection(ctx, schedule.Settings)
	finishedAt := time.Now().UnixMilli()
	state := accountInspectionStateCompleted
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			state = accountInspectionStateStopped
		} else if errors.Is(runErr, context.DeadlineExceeded) {
			state = accountInspectionStatePartial
		} else {
			state = accountInspectionStateFailed
		}
	}

	s.mu.Lock()
	s.setRunStateLocked(state)
	s.status.LastFinishedAt = finishedAt
	s.status.Summary = summary
	s.status.Results = results
	s.status.RestoredSnapshot = false
	s.healthCounts = accountInspectionResultHealthCounts(results)
	completed := s.status.Progress.Completed
	if state == accountInspectionStateCompleted {
		completed = len(results)
	} else if completed > len(results) {
		completed = len(results)
	}
	s.status.Progress.Completed = completed
	s.status.Progress.InFlight = 0
	s.status.Progress.Pending = 0
	if runErr != nil {
		s.status.LastError = runErr.Error()
	} else {
		s.status.LastError = ""
	}
	s.cancel = nil
	broadcast := s.statusBroadcastLocked()
	if s.schedule.Enabled && !manual {
		s.schedule.NextRunAt = time.Now().Add(time.Duration(s.schedule.IntervalMinutes) * time.Minute).UnixMilli()
		if err := s.saveLocked(); err != nil {
			log.WithError(err).Warn("failed to save next account inspection run time")
		}
	}
	if err := s.saveResultSnapshotLocked(); err != nil {
		log.WithError(err).Warn("failed to save account inspection snapshot")
	}
	s.mu.Unlock()
	broadcast.send()
}

func (s *accountInspectionScheduler) streamLogs(c *gin.Context) {
	responseHeader := http.Header{}
	for _, protocol := range strings.Split(c.GetHeader("Sec-WebSocket-Protocol"), ",") {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, "cpa-management.") {
			responseHeader.Set("Sec-WebSocket-Protocol", protocol)
			break
		}
	}
	conn, err := accountInspectionWebSocketUpgrader.Upgrade(c.Writer, c.Request, responseHeader)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go readAccountInspectionWebSocket(conn, done)

	subscriber, snapshot := s.subscribeLogs(accountInspectionRequestSnapshotOptions(c))
	defer s.unsubscribeLogs(subscriber)

	pingTicker := time.NewTicker(accountInspectionWebSocketPingPeriod)
	defer pingTicker.Stop()

	if err := writeAccountInspectionWebSocketMessage(conn, snapshot); err != nil {
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-done:
			return
		case <-pingTicker.C:
			if err := writeAccountInspectionWebSocketPing(conn); err != nil {
				return
			}
		case message, ok := <-subscriber:
			if !ok {
				return
			}
			if err := writeAccountInspectionWebSocketMessage(conn, message); err != nil {
				return
			}
		}
	}
}

func readAccountInspectionWebSocket(conn *websocket.Conn, done chan<- struct{}) {
	defer close(done)
	_ = conn.SetReadDeadline(time.Now().Add(accountInspectionWebSocketPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(accountInspectionWebSocketPongWait))
	})
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

func writeAccountInspectionWebSocketPing(conn *websocket.Conn) error {
	_ = conn.SetWriteDeadline(time.Now().Add(accountInspectionWebSocketWriteTimeout))
	return conn.WriteMessage(websocket.PingMessage, nil)
}

func writeAccountInspectionWebSocketMessage(conn *websocket.Conn, message accountInspectionLogStreamMessage) error {
	_ = conn.SetWriteDeadline(time.Now().Add(accountInspectionWebSocketWriteTimeout))
	return conn.WriteJSON(message)
}

func runAccountInspectionWorkers(total int, workers int, beforeNext func() bool, run func(index int) bool) {
	proinspection.RunWorkers(total, workers, beforeNext, run)
}

func (s *accountInspectionScheduler) executeInspection(ctx context.Context, settings accountInspectionSettings) ([]accountInspectionResult, accountInspectionSummary, error) {
	auths, err := s.auths()
	if err != nil {
		return nil, accountInspectionSummary{}, err
	}
	liveAuths := make([]*coreauth.Auth, 0, len(auths))
	accounts := make([]accountInspectionAccount, 0, len(auths))
	existingPaths := make(map[string]bool)
	for _, auth := range auths {
		liveAuths = append(liveAuths, auth)
		account := accountFromAuth(auth)
		if shouldInspectAccount(account, settings.TargetType) {
			accounts = append(accounts, account)
		}
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].FileName == accounts[j].FileName {
			return accounts[i].AuthIndex < accounts[j].AuthIndex
		}
		return accounts[i].FileName < accounts[j].FileName
	})
	probeSetCount := len(accounts)
	accounts = sampleAccounts(accounts, settings.SampleSize)
	accounts = s.filterExistingAccounts(accounts, existingPaths)
	s.appendLog("info", fmt.Sprintf("巡检集合 %d 个账号，本次探测 %d 个账号", probeSetCount, len(accounts)))

	results := make([]accountInspectionResult, len(accounts))
	providerLimiters := accountInspectionProviderLimiters()
	refreshLimiter := make(chan struct{}, accountInspectionMaxRefreshConcurrency)
	completed := 0
	inFlight := 0
	var progressMu sync.Mutex
	var runErr error
	var runErrOnce sync.Once
	setRunErr := func(err error) {
		if err == nil {
			return
		}
		runErrOnce.Do(func() { runErr = err })
	}
	s.updateProgress(len(accounts), 0, 0, true)
	runAccountInspectionWorkers(
		len(accounts),
		settings.Workers,
		func() bool {
			if err := s.waitIfPaused(ctx); err != nil {
				setRunErr(err)
				return false
			}
			return true
		},
		func(index int) bool {
			account := accounts[index]
			limiter := providerLimiters[account.Provider]
			if limiter == nil {
				limiter = make(chan struct{}, accountInspectionMaxProviderConcurrency)
			}
			select {
			case limiter <- struct{}{}:
			case <-ctx.Done():
				setRunErr(ctx.Err())
				return false
			}
			progressMu.Lock()
			inFlight++
			s.updateProgress(len(accounts), completed, inFlight, false)
			progressMu.Unlock()
			results[index] = s.inspectAccount(ctx, account, settings, refreshLimiter)
			<-limiter
			progressMu.Lock()
			inFlight--
			completed++
			s.updateProgress(len(accounts), completed, inFlight, false)
			progressMu.Unlock()
			return true
		},
	)
	if runErr != nil {
		partial := completedInspectionResults(results)
		return partial, summarizeAccountInspection(len(liveAuths), probeSetCount, accounts, partial), runErr
	}
	if err := ctx.Err(); err != nil {
		partial := completedInspectionResults(results)
		return partial, summarizeAccountInspection(len(liveAuths), probeSetCount, accounts, partial), err
	}

	s.applyAutomaticActions(ctx, results, settings)
	return results, summarizeAccountInspection(len(liveAuths), probeSetCount, accounts, results), nil
}

func completedInspectionResults(results []accountInspectionResult) []accountInspectionResult {
	return proinspection.CompletedResults(results)
}

func accountInspectionProviderLimiters() map[string]chan struct{} {
	return proinspection.ProviderLimiters(accountInspectionSupportedProviders, accountInspectionMaxProviderConcurrency)
}

func (s *accountInspectionScheduler) auths() ([]*coreauth.Auth, error) {
	if s.h == nil {
		return nil, fmt.Errorf("management handler unavailable")
	}
	s.h.mu.Lock()
	manager := s.h.authManager
	s.h.mu.Unlock()
	if manager == nil {
		return nil, fmt.Errorf("core auth manager unavailable")
	}
	return manager.List(), nil
}

func (s *accountInspectionScheduler) filterExistingAccounts(accounts []accountInspectionAccount, existingPaths map[string]bool) []accountInspectionAccount {
	out := accounts[:0]
	for _, account := range accounts {
		if s.authFileExists(account.Auth, existingPaths) {
			out = append(out, account)
		}
	}
	return out
}

func (s *accountInspectionScheduler) authFileExists(auth *coreauth.Auth, existingPaths map[string]bool) bool {
	if auth == nil {
		return false
	}
	if isRuntimeOnlyAuth(auth) {
		return true
	}
	path := strings.TrimSpace(authAttribute(auth, "path"))
	if path == "" && s.h != nil && s.h.cfg != nil {
		fileName := strings.TrimSpace(auth.FileName)
		if fileName != "" {
			path = filepath.Join(s.h.cfg.AuthDir, filepath.Base(fileName))
		}
	}
	if path == "" {
		return true
	}
	if exists, ok := existingPaths[path]; ok {
		return exists
	}
	_, err := os.Stat(path)
	exists := err == nil || !os.IsNotExist(err)
	existingPaths[path] = exists
	return exists
}

func accountInspectionKey(fileName string, authIndex string) string {
	return proinspection.AccountKey(fileName, authIndex)
}

func accountFromAuth(auth *coreauth.Auth) accountInspectionAccount {
	if auth == nil {
		return accountInspectionAccount{}
	}
	auth.EnsureIndex()
	provider := accountInspectionProvider(auth)
	fileName := strings.TrimSpace(auth.FileName)
	if fileName == "" {
		fileName = strings.TrimSpace(auth.ID)
	}
	name := firstNonEmptyAuthValue(auth, "name")
	email := accountInspectionAuthEmail(auth)
	displayName := firstNonEmptyStringValue(email, fileName)
	return accountInspectionAccount{
		Auth:        auth,
		Key:         accountInspectionKey(fileName, auth.Index),
		Provider:    provider,
		FileName:    fileName,
		DisplayName: displayName,
		Email:       email,
		Name:        name,
		AuthIndex:   auth.Index,
		Disabled:    auth.Disabled,
	}
}

func accountInspectionProvider(auth *coreauth.Auth) string {
	return strings.ToLower(strings.TrimSpace(auth.Provider))
}

func isAccountInspectionAPIKeyAuth(auth *coreauth.Auth) bool {
	if auth == nil {
		return false
	}
	label := strings.ToLower(strings.TrimSpace(auth.Label))
	if strings.Contains(label, "apikey") || strings.Contains(label, "api-key") {
		return true
	}
	source := strings.ToLower(strings.TrimSpace(authAttribute(auth, "source")))
	if strings.HasPrefix(source, "config:") && strings.TrimSpace(authAttribute(auth, "api_key")) != "" {
		return true
	}
	return strings.TrimSpace(authAttribute(auth, "api_key")) != "" && strings.TrimSpace(authAttribute(auth, "path")) == ""
}

func shouldInspectAccount(account accountInspectionAccount, targetType string) bool {
	return proinspection.ShouldInspectCandidate(account.Auth != nil, isAccountInspectionAPIKeyAuth(account.Auth), account.Provider, targetType)
}

func sampleAccounts(accounts []accountInspectionAccount, sampleSize int) []accountInspectionAccount {
	return proinspection.Sample(accounts, sampleSize, time.Now().UnixNano())
}

func (s *accountInspectionScheduler) inspectAccount(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings, refreshLimiter chan struct{}) accountInspectionResult {
	result := account.baseResult()
	if account.AuthIndex == "" {
		result.ActionReason = "缺少 auth_index，保留账号"
		result.Error = "missing auth_index"
		result.ErrorCode = "missing_auth_index"
		return result
	}
	if refreshed, refreshTriggered, refreshErr := s.refreshAccountIfDue(ctx, account, refreshLimiter); refreshErr != nil {
		result.TokenRefreshTriggered = refreshTriggered
		result.NextRefreshAt = account.nextRefreshAtMillis()
		if errors.Is(refreshErr, context.Canceled) || errors.Is(refreshErr, context.DeadlineExceeded) {
			result.Error = refreshErr.Error()
			result.ActionReason = "巡检已取消，保留账号"
			return result
		}
		result.TokenRefreshStatus = "failed"
		result.TokenRefreshError = refreshErr.Error()
		result.Error = refreshErr.Error()
		result.ErrorCode = "token_refresh_error"
		result.ActionReason = "刷新令牌失败，保留账号"
		s.syncInspectionAuthError(ctx, account, "token_refresh_error", refreshErr.Error(), 0)
		s.appendLog("warning", fmt.Sprintf("%s 刷新令牌失败，保留账号：%s", account.identity(), refreshErr.Error()))
		return result
	} else if refreshTriggered {
		account = refreshed
		result = account.baseResult()
		result.TokenRefreshTriggered = true
		result.TokenRefreshStatus = "success"
	} else if refreshed.Auth != nil {
		account = refreshed
		result = account.baseResult()
	}
	result.NextRefreshAt = account.nextRefreshAtMillis()
	var decision accountInspectionDecision
	var statusCode *int
	var err error
	switch account.Provider {
	case "antigravity":
		decision, statusCode, err = s.inspectAntigravity(ctx, account, settings)
	case "claude":
		decision, statusCode, err = s.inspectClaude(ctx, account, settings)
	case "codex":
		decision, statusCode, err = s.inspectCodex(ctx, account, settings)
	case "gemini-cli":
		decision, statusCode, err = s.inspectGeminiCLI(ctx, account, settings)
	case "kimi":
		decision, statusCode, err = s.inspectKimi(ctx, account, settings)
	case "xai":
		decision, statusCode, err = s.inspectXAI(ctx, account, settings)
	default:
		result.ActionReason = "暂不支持该 provider 巡检"
		result.Error = "unsupported provider"
		return result
	}
	if err != nil {
		result.StatusCode = statusCode
		result.Error = err.Error()
		result.ErrorCode = accountInspectionErrorCode(statusCode, "inspection_probe_error")
		result.ActionReason = "探测异常，保留账号"
		if statusCode != nil && isAccountErrorStatus(*statusCode) {
			s.syncInspectionAuthStatus(ctx, account, *statusCode)
		} else {
			s.syncInspectionAuthError(ctx, account, "inspection_probe_error", err.Error(), 0)
		}
		s.appendLog("warning", fmt.Sprintf("%s 探测异常，保留账号：%s", account.identity(), err.Error()))
		return result
	}
	result.StatusCode = statusCode
	result.Action = decision.Action
	result.ActionReason = decision.ActionReason
	result.UsedPercent = decision.UsedPercent
	result.IsQuota = decision.IsQuota
	result.Error = decision.Error
	result.ErrorDetail = decision.ErrorDetail
	result.ErrorCode = accountInspectionDecisionErrorCode(account.Provider, decision, statusCode)
	if decision.DeepProbeStatus != "" {
		result.DeepProbeTriggered = true
		result.DeepProbeStatus = string(decision.DeepProbeStatus)
		result.DeepProbeError = decision.DeepProbeError
	}
	if decision.IsQuota {
		s.clearInspectionAuthError(ctx, account)
	} else if statusCode != nil && decision.DeepProbeStatus != accountInspectionDeepProbeTransientError {
		s.syncInspectionAuthStatus(ctx, account, *statusCode)
	}
	level := "info"
	if result.Action == accountInspectionActionDisable {
		level = "warning"
	} else if result.Action == accountInspectionActionEnable {
		level = "success"
	} else if result.Action == accountInspectionActionDelete {
		level = "error"
	}
	percent := "--"
	if result.UsedPercent != nil {
		percent = fmt.Sprintf("%.1f%%", *result.UsedPercent)
	}
	s.appendLog(level, fmt.Sprintf("%s -> %s (%s · 已用 %s)", account.identity(), result.Action, account.Provider, percent))
	return result
}

func (s *accountInspectionScheduler) refreshAccountIfDue(ctx context.Context, account accountInspectionAccount, refreshLimiter chan struct{}) (accountInspectionAccount, bool, error) {
	if account.Auth == nil || account.Auth.ID == "" || s == nil || s.h == nil || s.h.authManager == nil {
		return account, false, nil
	}
	if refreshLimiter != nil {
		select {
		case refreshLimiter <- struct{}{}:
			defer func() { <-refreshLimiter }()
		case <-ctx.Done():
			return account, false, ctx.Err()
		}
	}
	updated, refreshed, err := s.h.authManager.RefreshIfDueForInspection(ctx, account.Auth.ID)
	if err != nil {
		return account, true, err
	}
	if updated == nil {
		return account, false, nil
	}
	refreshedAccount := accountFromAuth(updated)
	if refreshed {
		s.appendLog("success", fmt.Sprintf("%s 刷新令牌成功", refreshedAccount.identity()))
	}
	return refreshedAccount, refreshed, nil
}

func (account accountInspectionAccount) nextRefreshAtMillis() int64 {
	if account.Auth == nil || account.Auth.NextRefreshAfter.IsZero() {
		return 0
	}
	return account.Auth.NextRefreshAfter.UnixMilli()
}

func (account accountInspectionAccount) baseResult() accountInspectionResult {
	return accountInspectionResult{
		Key:          account.Key,
		Provider:     account.Provider,
		FileName:     account.FileName,
		DisplayName:  account.DisplayName,
		Email:        account.Email,
		Name:         account.Name,
		AuthIndex:    account.AuthIndex,
		Disabled:     account.Disabled,
		Action:       accountInspectionActionKeep,
		ActionReason: "无需处理",
	}
}

func formatAccountInspectionIdentity(fileName string, email string, name string, displayName string) string {
	label := firstNonEmptyStringValue(email, name, displayName)
	if label != "" && label != "-" {
		if fileName != "" {
			return fmt.Sprintf("%s[%s]", label, fileName)
		}
		return label
	}
	return fileName
}

func (account accountInspectionAccount) identity() string {
	return formatAccountInspectionIdentity(account.FileName, account.Email, account.Name, account.DisplayName)
}

func (s *accountInspectionScheduler) apiCall(ctx context.Context, auth *coreauth.Auth, method string, url string, headers map[string]string, data string, timeoutMS int) (accountInspectionHTTPResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeoutMS <= 0 {
		timeoutMS = accountInspectionDefaultTimeoutMS
	}
	var body io.Reader
	if data != "" {
		body = bytes.NewBufferString(data)
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, url, body)
	if err != nil {
		return accountInspectionHTTPResult{}, err
	}
	resolvedHeaders := make(map[string]string, len(headers))
	var token string
	var tokenResolved bool
	for key, value := range headers {
		if strings.Contains(value, "$TOKEN$") {
			if !tokenResolved {
				token, err = s.h.resolveTokenForAuth(reqCtx, auth)
				tokenResolved = true
				if err != nil {
					return accountInspectionHTTPResult{}, err
				}
			}
			value = strings.ReplaceAll(value, "$TOKEN$", token)
		}
		resolvedHeaders[key] = value
	}
	for key, value := range resolvedHeaders {
		req.Header.Set(key, value)
	}
	if accountInspectionShouldUseExecutorHTTPRequest(auth) {
		if s == nil || s.h == nil || s.h.authManager == nil {
			return accountInspectionHTTPResult{}, fmt.Errorf("core auth manager unavailable")
		}
		resp, err := s.h.authManager.HttpRequest(reqCtx, auth, req)
		if err != nil {
			return accountInspectionHTTPResult{}, err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		return accountInspectionHTTPResult{StatusCode: resp.StatusCode, Body: string(raw), Header: resp.Header.Clone()}, nil
	}
	client := &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond, Transport: s.h.apiCallTransport(auth)}
	resp, err := client.Do(req)
	if err != nil {
		return accountInspectionHTTPResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return accountInspectionHTTPResult{StatusCode: resp.StatusCode, Body: string(raw), Header: resp.Header.Clone()}, nil
}

func accountInspectionShouldUseExecutorHTTPRequest(auth *coreauth.Auth) bool {
	if auth == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(auth.Provider)) {
	case "gemini-cli", "xai":
		return true
	default:
		return false
	}
}

func (s *accountInspectionScheduler) withRetry(ctx context.Context, retries int, task func() (accountInspectionHTTPResult, error)) (accountInspectionHTTPResult, error) {
	var last accountInspectionHTTPResult
	var err error
	for i := 0; i <= retries; i++ {
		last, err = task()
		if err == nil {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		default:
		}
	}
	return last, err
}

func (s *accountInspectionScheduler) inspectAntigravity(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	projectID := antigravityProjectID(account.Auth)
	body := `{"project":"` + escapeJSONString(projectID) + `"}`
	urls := antigravityQuotaURLs()
	var priorityStatus *int
	var priorityDetail string
	for _, url := range urls {
		resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
			return s.apiCall(ctx, account.Auth, http.MethodPost, url, map[string]string{
				"Authorization": "Bearer $TOKEN$",
				"Content-Type":  "application/json",
				"User-Agent":    s.antigravityUserAgent(),
			}, body, settings.Timeout)
		})
		if err != nil {
			continue
		}
		status := intPtr(resp.StatusCode)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if isQuotaHTTPStatus(resp.StatusCode) || isAntigravityQuotaFailure(resp.Body) {
				return quotaUnavailableDecision(account, "Antigravity 额度不可用，建议禁用账号", resp.Body), status, nil
			}
			if isAccountErrorStatus(resp.StatusCode) {
				priorityStatus = status
				priorityDetail = resp.Body
			}
			continue
		}
		groups, err := buildAntigravityGroups(resp.Body)
		if err != nil {
			continue
		}
		quotaState := map[string]any{"groups": groups, "rawShapeHash": jsonShapeHash(resp.Body)}
		if subscription := s.fetchAntigravitySubscription(ctx, account, settings); subscription != nil {
			quotaState["subscription"] = subscription
			if plan := stringFromAny(subscription["plan"]); plan != "" {
				quotaState["plan"] = plan
				quotaState["planType"] = plan
			}
		}
		s.persistQuotaState(ctx, account, quotaSuccessState(quotaState))
		used := antigravityUsedPercent(groups, settings.AntigravityQuotaMode)
		decision := quotaDecision(account, used, used != nil, settings.UsedPercentThreshold)
		if settings.AntigravityDeepProbeEnabled && antigravityShouldDeepProbe(decision) {
			return s.applyAntigravityDeepProbe(ctx, account, settings, groups, decision, status)
		}
		return decision, status, nil
	}
	if priorityStatus != nil {
		return withInspectionHTTPErrorDetail(authErrorDecision(account, *priorityStatus), priorityDetail), priorityStatus, nil
	}
	return accountInspectionDecision{}, priorityStatus, fmt.Errorf("antigravity quota unavailable")
}

func antigravityQuotaURLs() []string {
	return []string{
		"https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary",
		"https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary",
		"https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary",
	}
}

func antigravityGenerateURLs() []string {
	return []string{
		"https://daily-cloudcode-pa.googleapis.com/v1internal:generateContent",
		"https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:generateContent",
		"https://cloudcode-pa.googleapis.com/v1internal:generateContent",
	}
}

func (s *accountInspectionScheduler) fetchAntigravitySubscription(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) map[string]any {
	resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodPost, antigravityCodeAssistURL, map[string]string{
			"Authorization": "Bearer $TOKEN$",
			"Content-Type":  "application/json",
			"User-Agent":    s.antigravityUserAgent(),
		}, `{"metadata":{"ideType":"ANTIGRAVITY"}}`, settings.Timeout)
	})
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	payload, err := parseAntigravityQuotaPayload(resp.Body)
	if err != nil {
		return nil
	}
	return buildAntigravitySubscription(payload)
}

func antigravityShouldDeepProbe(decision accountInspectionDecision) bool {
	return proinspection.ShouldAntigravityDeepProbe(decision)
}

func (s *accountInspectionScheduler) applyAntigravityDeepProbe(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings, groups []map[string]any, decision accountInspectionDecision, quotaStatus *int) (accountInspectionDecision, *int, error) {
	model := selectAntigravityDeepProbeModel(groups, settings.AntigravityDeepProbeModel)
	projectID := antigravityProjectID(account.Auth)
	if model == "" || projectID == "" {
		decision.DeepProbeStatus = accountInspectionDeepProbeSkipped
		if model == "" {
			decision.DeepProbeError = "no available Claude/GPT model for deep probe"
		} else {
			decision.DeepProbeError = "missing Antigravity project id"
		}
		s.appendLog("warning", fmt.Sprintf("%s Antigravity 深度检测跳过：%s", account.identity(), decision.DeepProbeError))
		return decision, quotaStatus, nil
	}

	s.appendLog("info", fmt.Sprintf("%s Antigravity 深度检测开始：%s", account.identity(), model))
	body := buildAntigravityDeepProbeBody(projectID, model)
	var lastStatus *int
	var lastMessage string
	var lastDetail string
	for _, url := range antigravityGenerateURLs() {
		resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
			return s.apiCall(ctx, account.Auth, http.MethodPost, url, map[string]string{
				"Authorization": "Bearer $TOKEN$",
				"Content-Type":  "application/json",
				"User-Agent":    s.antigravityUserAgent(),
			}, body, settings.Timeout)
		})
		if err != nil {
			lastMessage = err.Error()
			continue
		}
		lastStatus = intPtr(resp.StatusCode)
		probeStatus, probeMessage := classifyAntigravityDeepProbeResponse(resp)
		probeDetail := inspectionHTTPErrorDetail(resp.Body)
		switch probeStatus {
		case accountInspectionDeepProbeSuccess:
			s.clearInspectionAuthError(ctx, account)
			decision.DeepProbeStatus = accountInspectionDeepProbeSuccess
			decision.DeepProbeError = ""
			s.appendLog("success", fmt.Sprintf("%s Antigravity 深度检测通过", account.identity()))
			return decision, lastStatus, nil
		case accountInspectionDeepProbeAuthError:
			s.syncInspectionAuthStatus(ctx, account, resp.StatusCode)
			probeDecision := authErrorDecision(account, resp.StatusCode)
			probeDecision.UsedPercent = decision.UsedPercent
			probeDecision.DeepProbeStatus = accountInspectionDeepProbeAuthError
			probeDecision.DeepProbeError = probeMessage
			probeDecision.ErrorDetail = probeDetail
			s.appendLog("warning", fmt.Sprintf("%s Antigravity 深度检测授权异常：%s", account.identity(), probeMessage))
			return probeDecision, lastStatus, nil
		case accountInspectionDeepProbeQuota:
			s.clearInspectionAuthError(ctx, account)
			probeDecision := accountInspectionDecision{Action: accountInspectionActionDisable, ActionReason: "Antigravity 深度检测返回额度不可用，建议禁用账号", UsedPercent: decision.UsedPercent, IsQuota: true, ErrorDetail: probeDetail, DeepProbeStatus: accountInspectionDeepProbeQuota, DeepProbeError: probeMessage}
			if account.Disabled {
				probeDecision.Action = accountInspectionActionKeep
				probeDecision.ActionReason = "Antigravity 深度检测返回额度不可用，但账号已禁用"
			}
			s.appendLog("warning", fmt.Sprintf("%s Antigravity 深度检测额度不可用：%s", account.identity(), probeMessage))
			return probeDecision, lastStatus, nil
		default:
			lastMessage = probeMessage
			lastDetail = probeDetail
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < http.StatusInternalServerError {
				break
			}
		}
	}
	if lastMessage == "" {
		lastMessage = "antigravity deep probe unavailable"
	}
	s.syncInspectionAuthError(ctx, account, "antigravity_deep_probe_error", lastMessage, statusValue(lastStatus))
	decision.Action = accountInspectionActionKeep
	decision.ActionReason = "Antigravity 深度检测临时异常，保留账号"
	decision.Error = lastMessage
	decision.ErrorDetail = lastDetail
	decision.DeepProbeStatus = accountInspectionDeepProbeTransientError
	decision.DeepProbeError = lastMessage
	s.appendLog("warning", fmt.Sprintf("%s Antigravity 深度检测临时异常：%s", account.identity(), lastMessage))
	return decision, firstStatus(lastStatus, quotaStatus), nil
}

func selectAntigravityDeepProbeModel(groups []map[string]any, preferredModel string) string {
	return proinspection.SelectAntigravityDeepProbeModel(preferredModel)
}

func buildAntigravityDeepProbeBody(projectID string, model string) string {
	return proinspection.BuildAntigravityDeepProbeBody(projectID, model)
}

func classifyAntigravityDeepProbeResponse(resp accountInspectionHTTPResult) (accountInspectionDeepProbeStatus, string) {
	return proinspection.ClassifyAntigravityDeepProbeResponse(resp)
}

func hasAntigravityGenerateContent(body string) bool {
	return proinspection.HasAntigravityGenerateContent(body)
}

func isAntigravityQuotaFailure(body string) bool {
	return proinspection.IsAntigravityQuotaFailure(body)
}

func summarizeInspectionHTTPBody(body string) string {
	return proinspection.SummarizeHTTPBody(body)
}

func inspectionHTTPErrorDetail(body string) string {
	return proinspection.HTTPErrorDetail(body)
}

func withInspectionHTTPErrorDetail(decision accountInspectionDecision, body string) accountInspectionDecision {
	return proinspection.WithHTTPErrorDetail(decision, body)
}

func statusValue(status *int) int {
	return proinspection.StatusValue(status)
}

func firstStatus(primary *int, fallback *int) *int {
	return proinspection.FirstStatus(primary, fallback)
}

func (s *accountInspectionScheduler) inspectClaude(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	usageResp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", s.claudeHeaders(), "", settings.Timeout)
	})
	status := intPtr(usageResp.StatusCode)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	if usageResp.StatusCode < 200 || usageResp.StatusCode >= 300 {
		if isQuotaHTTPStatus(usageResp.StatusCode) {
			return quotaUnavailableDecision(account, "Claude 额度不可用，建议禁用账号", usageResp.Body), status, nil
		}
		if isAccountErrorStatus(usageResp.StatusCode) {
			return withInspectionHTTPErrorDetail(authErrorDecision(account, usageResp.StatusCode), usageResp.Body), status, nil
		}
		return accountInspectionDecision{}, status, fmt.Errorf("HTTP %d", usageResp.StatusCode)
	}
	windows, extraUsage, err := buildClaudeWindows(usageResp.Body)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	planType := ""
	profileResp, profileErr := s.apiCall(ctx, account.Auth, http.MethodGet, "https://api.anthropic.com/api/oauth/profile", s.claudeHeaders(), "", settings.Timeout)
	if profileErr == nil && profileResp.StatusCode >= 200 && profileResp.StatusCode < 300 {
		planType = resolveClaudePlan(profileResp.Body)
	}
	s.persistQuotaState(ctx, account, quotaSuccessState(map[string]any{"windows": windows, "extraUsage": extraUsage, "planType": emptyStringAsNil(planType), "rawShapeHash": jsonShapeHash(usageResp.Body)}))
	used := maxUsedPercentFromWindows(windows)
	return quotaDecision(account, used, len(windows) > 0, settings.UsedPercentThreshold), status, nil
}

func (s *accountInspectionScheduler) inspectCodex(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	accountID := codexAccountID(account.Auth)
	if accountID == "" {
		return accountInspectionDecision{}, nil, fmt.Errorf("missing ChatGPT account id")
	}
	resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", map[string]string{
			"Authorization":      "Bearer $TOKEN$",
			"Content-Type":       "application/json",
			"User-Agent":         s.codexUserAgent(),
			"Chatgpt-Account-Id": accountID,
		}, "", settings.Timeout)
	})
	status := intPtr(resp.StatusCode)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	payload, windows, used := buildCodexWindows(resp.Body)
	isQuota := isQuotaHTTPStatus(resp.StatusCode) || strings.Contains(strings.ToLower(resp.Body), "quota exhausted") || strings.Contains(strings.ToLower(resp.Body), "limit reached") || strings.Contains(strings.ToLower(resp.Body), "payment_required")
	if used != nil && *used >= float64(settings.UsedPercentThreshold) {
		isQuota = true
	}
	if payload != nil && len(windows) > 0 {
		s.persistQuotaState(ctx, account, quotaSuccessState(codexQuotaStateValues(account.Auth, payload, windows, resp.Body)))
	}
	decision := codexDecision(account, resp.StatusCode, used, isQuota, settings.UsedPercentThreshold)
	if isAccountErrorStatus(resp.StatusCode) {
		decision = withInspectionHTTPErrorDetail(decision, resp.Body)
	}
	return decision, status, nil
}

func (s *accountInspectionScheduler) inspectGeminiCLI(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	if s == nil || s.quota == nil {
		return accountInspectionDecision{}, intPtr(http.StatusServiceUnavailable), fmt.Errorf("quota gateway unavailable")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.Timeout)*time.Millisecond)
	result, err := s.quota.FetchQuota(attemptCtx, account.AuthIndex)
	cancel()
	for attempt := 0; err != nil && attempt < settings.Retries && ctx.Err() == nil; attempt++ {
		attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(settings.Timeout)*time.Millisecond)
		result, err = s.quota.FetchQuota(attemptCtx, account.AuthIndex)
		cancel()
	}
	upstreamStatus := result.UpstreamStatus
	if err != nil {
		status := upstreamStatus
		if status == 0 {
			status = result.ServiceStatus
		}
		if isQuotaHTTPStatus(upstreamStatus) {
			return quotaUnavailableDecision(account, "Gemini CLI 额度不可用，建议禁用账号", ""), intPtr(upstreamStatus), nil
		}
		if isAccountErrorStatus(upstreamStatus) {
			return authErrorDecision(account, upstreamStatus), intPtr(upstreamStatus), nil
		}
		return accountInspectionDecision{}, intPtr(status), err
	}
	if errCleanup := s.cleanupLegacyQuotaCacheFromAuth(ctx, account); errCleanup != nil {
		s.appendLog("warning", fmt.Sprintf("%s 旧认证文件配额缓存清理失败：%s", account.identity(), errCleanup.Error()))
	}
	used, hasQuota := quotaSnapshotMaxUsedPercent(result.Snapshot)
	return quotaDecision(account, used, hasQuota, settings.UsedPercentThreshold), intPtr(http.StatusOK), nil
}

func quotaSnapshotMaxUsedPercent(snapshot pluginapi.QuotaSnapshot) (*float64, bool) {
	return proquota.SnapshotMaxUsedPercent(snapshot)
}

func (s *accountInspectionScheduler) inspectKimi(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodGet, "https://api.kimi.com/coding/v1/usages", map[string]string{"Authorization": "Bearer $TOKEN$"}, "", settings.Timeout)
	})
	status := intPtr(resp.StatusCode)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if isQuotaHTTPStatus(resp.StatusCode) {
			return quotaUnavailableDecision(account, "Kimi 额度不可用，建议禁用账号", resp.Body), status, nil
		}
		if isAccountErrorStatus(resp.StatusCode) {
			return withInspectionHTTPErrorDetail(authErrorDecision(account, resp.StatusCode), resp.Body), status, nil
		}
		return accountInspectionDecision{}, status, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	rows, used, err := buildKimiRows(resp.Body)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	s.persistQuotaState(ctx, account, quotaSuccessState(map[string]any{"rows": rows, "rawShapeHash": jsonShapeHash(resp.Body)}))
	return quotaDecision(account, used, len(rows) > 0, settings.UsedPercentThreshold), status, nil
}

func (s *accountInspectionScheduler) inspectXAI(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	if xaiInspectionUsingAPI(account.Auth) {
		return s.inspectXAIOfficialAPI(ctx, account, settings)
	}
	return s.inspectXAICLI(ctx, account, settings)
}

func (s *accountInspectionScheduler) inspectXAIOfficialAPI(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	model := strings.TrimSpace(settings.XAIDeepProbeModel)
	if model == "" {
		model = "grok-4.5"
	}
	resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodPost, xaiOfficialChatURL(account.Auth), xaiOfficialAPIHeaders(), buildXAIOfficialHealthBody(model), settings.Timeout)
	})
	status := intPtr(resp.StatusCode)
	if err != nil {
		return accountInspectionDecision{}, status, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if isQuotaHTTPStatus(resp.StatusCode) || isXAIQuotaFailure(resp.Body) {
			return xaiOfficialAPIQuotaDecision(account, resp.Body), status, nil
		}
		if isAccountErrorStatus(resp.StatusCode) {
			return withInspectionHTTPErrorDetail(authErrorDecision(account, resp.StatusCode), resp.Body), status, nil
		}
		return accountInspectionDecision{}, status, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	billing := xaiPaidHealthSummary()
	s.persistQuotaState(ctx, account, quotaSuccessState(map[string]any{
		"billing":      billing,
		"rawShapeHash": jsonShapeHash(resp.Body),
	}))
	decision := healthyDecision(account)
	if settings.XAIDeepProbeEnabled {
		return s.applyXAIDeepProbe(ctx, account, settings, decision, status)
	}
	return decision, status, nil
}

func (s *accountInspectionScheduler) inspectXAICLI(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings) (accountInspectionDecision, *int, error) {
	headers := xaiRequestHeaders(account.Auth)
	weeklyBilling, weeklyResp, weeklyErr := s.fetchXAIBillingSummary(ctx, account, settings, xaiBillingWeeklyURL(), headers)
	monthlyBilling, monthlyResp, monthlyErr := s.fetchXAIBillingSummary(ctx, account, settings, xaiBillingURL(), headers)
	status := firstInspectionStatus(monthlyResp.StatusCode, weeklyResp.StatusCode)
	billing := mergeXAIBillingSummaries(weeklyBilling, monthlyBilling)
	if planType, known := xaiPlanTypeFromBillingBody(monthlyResp.StatusCode, monthlyResp.Body); known {
		if billing == nil {
			billing = emptyXAIBillingSummary()
		}
		billing["planType"] = planType
	}
	if billing == nil {
		if isQuotaHTTPStatus(weeklyResp.StatusCode) || isXAIQuotaFailure(weeklyResp.Body) {
			return quotaUnavailableDecision(account, "xAI 额度不可用，建议禁用账号", weeklyResp.Body), status, nil
		}
		if isQuotaHTTPStatus(monthlyResp.StatusCode) || isXAIQuotaFailure(monthlyResp.Body) {
			return quotaUnavailableDecision(account, "xAI 额度不可用，建议禁用账号", monthlyResp.Body), status, nil
		}
		if isAccountErrorStatus(weeklyResp.StatusCode) {
			return withInspectionHTTPErrorDetail(authErrorDecision(account, weeklyResp.StatusCode), weeklyResp.Body), status, nil
		}
		if isAccountErrorStatus(monthlyResp.StatusCode) {
			return withInspectionHTTPErrorDetail(authErrorDecision(account, monthlyResp.StatusCode), monthlyResp.Body), status, nil
		}
	}
	billing = mergeCachedXAIFreeQuota(ctx, account, billing)
	if billing == nil {
		if weeklyErr != nil {
			return accountInspectionDecision{}, status, weeklyErr
		}
		if monthlyErr != nil {
			return accountInspectionDecision{}, status, monthlyErr
		}
		return accountInspectionDecision{}, status, fmt.Errorf("empty xai billing config")
	}
	used := xaiSummaryUsedPercent(billing)
	s.persistQuotaState(ctx, account, quotaSuccessState(map[string]any{
		"billing":             billing,
		"rawShapeHash":        jsonShapeHashForBodies(map[string]string{"weekly": weeklyResp.Body, "monthly": monthlyResp.Body}),
		"weeklyRawShapeHash":  jsonShapeHash(weeklyResp.Body),
		"monthlyRawShapeHash": jsonShapeHash(monthlyResp.Body),
	}))
	decision := quotaDecision(account, used, billing != nil, settings.UsedPercentThreshold)
	if settings.XAIDeepProbeEnabled && accountInspectionShouldDeepProbe(decision) {
		return s.applyXAIDeepProbe(ctx, account, settings, decision, status)
	}
	return decision, status, nil
}

func accountInspectionShouldDeepProbe(decision accountInspectionDecision) bool {
	return proinspection.ShouldDeepProbe(decision)
}

func (s *accountInspectionScheduler) applyXAIDeepProbe(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings, decision accountInspectionDecision, quotaStatus *int) (accountInspectionDecision, *int, error) {
	model := strings.TrimSpace(settings.XAIDeepProbeModel)
	if model == "" {
		decision.DeepProbeStatus = accountInspectionDeepProbeSkipped
		decision.DeepProbeError = "missing xAI deep probe model"
		s.appendLog("warning", fmt.Sprintf("%s xAI 深度检测跳过：%s", account.identity(), decision.DeepProbeError))
		return decision, quotaStatus, nil
	}

	release, err := s.acquireXAIDeepProbe(ctx)
	if err != nil {
		return decision, quotaStatus, err
	}
	defer release()

	s.appendLog("info", fmt.Sprintf("%s xAI 深度检测开始：%s", account.identity(), model))
	resp, status, message, err := runXAIDeepProbeWithRetry(ctx, settings.Retries, accountInspectionXAIRetryDelay, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodPost, xaiResponsesURL(account.Auth), xaiDeepProbeHeaders(account.Auth), buildXAIDeepProbeBody(model), settings.Timeout)
	})
	if !xaiInspectionUsingAPI(account.Auth) {
		observeAccountXAIQuota(ctx, account, model, resp)
	}
	var probeStatus *int
	if resp.StatusCode != 0 {
		probeStatus = intPtr(resp.StatusCode)
	}
	if err != nil {
		message := err.Error()
		s.syncInspectionAuthError(ctx, account, "xai_deep_probe_error", message, statusValue(probeStatus))
		decision.Action = accountInspectionActionKeep
		decision.ActionReason = "xAI 深度检测临时异常，保留账号"
		decision.Error = message
		decision.DeepProbeStatus = accountInspectionDeepProbeTransientError
		decision.DeepProbeError = message
		s.appendLog("warning", fmt.Sprintf("%s xAI 深度检测临时异常：%s", account.identity(), message))
		return decision, firstStatus(probeStatus, quotaStatus), nil
	}

	errorDetail := inspectionHTTPErrorDetail(resp.Body)
	switch status {
	case accountInspectionDeepProbeSuccess:
		s.clearInspectionAuthError(ctx, account)
		decision.DeepProbeStatus = accountInspectionDeepProbeSuccess
		decision.DeepProbeError = ""
		s.appendLog("success", fmt.Sprintf("%s xAI 深度检测通过", account.identity()))
		return decision, probeStatus, nil
	case accountInspectionDeepProbeAuthError:
		s.syncInspectionAuthStatus(ctx, account, resp.StatusCode)
		probeDecision := authErrorDecision(account, resp.StatusCode)
		probeDecision.UsedPercent = decision.UsedPercent
		probeDecision.DeepProbeStatus = accountInspectionDeepProbeAuthError
		probeDecision.DeepProbeError = message
		probeDecision.ErrorDetail = errorDetail
		s.appendLog("warning", fmt.Sprintf("%s xAI 深度检测授权异常：%s", account.identity(), message))
		return probeDecision, probeStatus, nil
	case accountInspectionDeepProbeQuota:
		s.clearInspectionAuthError(ctx, account)
		probeDecision := accountInspectionDecision{Action: accountInspectionActionDisable, ActionReason: "xAI 深度检测返回额度不可用，建议禁用账号", UsedPercent: decision.UsedPercent, IsQuota: true, ErrorDetail: errorDetail, DeepProbeStatus: accountInspectionDeepProbeQuota, DeepProbeError: message}
		if account.Disabled {
			probeDecision.Action = accountInspectionActionKeep
			probeDecision.ActionReason = "xAI 深度检测返回额度不可用，但账号已禁用"
		}
		s.appendLog("warning", fmt.Sprintf("%s xAI 深度检测额度不可用：%s", account.identity(), message))
		return probeDecision, probeStatus, nil
	default:
		s.syncInspectionAuthError(ctx, account, "xai_deep_probe_error", message, resp.StatusCode)
		decision.Action = accountInspectionActionKeep
		decision.ActionReason = "xAI 深度检测临时异常，保留账号"
		decision.Error = message
		decision.ErrorDetail = errorDetail
		decision.DeepProbeStatus = accountInspectionDeepProbeTransientError
		decision.DeepProbeError = message
		s.appendLog("warning", fmt.Sprintf("%s xAI 深度检测临时异常：%s", account.identity(), message))
		return decision, probeStatus, nil
	}
}

func (s *accountInspectionScheduler) acquireXAIDeepProbe(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.xaiDeepProbeOnce.Do(func() {
		s.xaiDeepProbeGate = make(chan struct{}, 1)
	})
	select {
	case s.xaiDeepProbeGate <- struct{}{}:
		return func() { <-s.xaiDeepProbeGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runXAIDeepProbeWithRetry(
	ctx context.Context,
	retries int,
	retryDelay time.Duration,
	task func() (accountInspectionHTTPResult, error),
) (accountInspectionHTTPResult, accountInspectionDeepProbeStatus, string, error) {
	return proinspection.RunXAIDeepProbeWithRetry(ctx, retries, retryDelay, task)
}

func shouldRetryXAIDeepProbe(status accountInspectionDeepProbeStatus, message string) bool {
	return proinspection.ShouldRetryXAIDeepProbe(status, message)
}

func xaiInspectionBaseURL(auth *coreauth.Auth) string {
	baseURL := ""
	if auth != nil {
		baseURL = strings.TrimSpace(auth.Attributes["base_url"])
		if baseURL == "" {
			baseURL = strings.TrimSpace(stringFromAny(auth.Metadata["base_url"]))
		}
	}
	if !xaiInspectionUsingAPI(auth) && (baseURL == "" || strings.EqualFold(strings.TrimRight(baseURL, "/"), "https://api.x.ai/v1")) {
		baseURL = "https://cli-chat-proxy.grok.com/v1"
	} else if baseURL == "" {
		baseURL = "https://api.x.ai/v1"
	}
	return strings.TrimRight(baseURL, "/")
}

func xaiResponsesURL(auth *coreauth.Auth) string {
	return xaiInspectionBaseURL(auth) + "/responses"
}

func xaiOfficialChatURL(auth *coreauth.Auth) string {
	return xaiInspectionBaseURL(auth) + "/chat/completions"
}

func xaiInspectionUsingAPI(auth *coreauth.Auth) bool {
	if auth == nil {
		return true
	}
	if raw := strings.TrimSpace(auth.Attributes["using_api"]); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			return parsed
		}
	}
	if raw, ok := auth.Metadata["using_api"]; ok && raw != nil {
		switch value := raw.(type) {
		case bool:
			return value
		case string:
			if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
				return parsed
			}
		}
	}
	if authKind := strings.TrimSpace(auth.Attributes["auth_kind"]); authKind != "" {
		return !strings.EqualFold(authKind, "oauth")
	}
	return !strings.EqualFold(strings.TrimSpace(stringFromAny(auth.Metadata["auth_kind"])), "oauth")
}

func xaiDeepProbeHeaders(auth *coreauth.Auth) map[string]string {
	headers := map[string]string{"Authorization": "Bearer $TOKEN$"}
	if !xaiInspectionUsingAPI(auth) && strings.EqualFold(xaiInspectionBaseURL(auth), "https://cli-chat-proxy.grok.com/v1") {
		headers = xaiRequestHeaders(auth)
	}
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "text/event-stream"
	headers["Connection"] = "Keep-Alive"
	return headers
}

func xaiOfficialAPIHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Connection":    "Keep-Alive",
	}
}

func buildXAIOfficialHealthBody(model string) string {
	return proinspection.BuildXAIOfficialHealthBody(model)
}

func xaiPaidHealthSummary() map[string]any {
	return proquota.XAIPaidHealthSummary()
}

func xaiOfficialAPIQuotaDecision(account accountInspectionAccount, body string) accountInspectionDecision {
	return proinspection.XAIOfficialAPIQuotaDecision(account.Disabled, body)
}

func buildXAIDeepProbeBody(model string) string {
	return proinspection.BuildXAIDeepProbeBody(model)
}

func classifyXAIDeepProbeResponse(resp accountInspectionHTTPResult) (accountInspectionDeepProbeStatus, string) {
	return proinspection.ClassifyXAIDeepProbeResponse(resp)
}

func classifyXAIDeepProbeSuccessBody(body string) (accountInspectionDeepProbeStatus, string) {
	return proinspection.ClassifyXAIDeepProbeSuccessBody(body)
}

func isXAIQuotaFailure(body string) bool {
	return proinspection.IsXAIQuotaFailure(body)
}

func (s *accountInspectionScheduler) fetchXAIBillingSummary(ctx context.Context, account accountInspectionAccount, settings accountInspectionSettings, url string, headers map[string]string) (map[string]any, accountInspectionHTTPResult, error) {
	resp, err := s.withRetry(ctx, settings.Retries, func() (accountInspectionHTTPResult, error) {
		return s.apiCall(ctx, account.Auth, http.MethodGet, url, headers, "", settings.Timeout)
	})
	if err != nil {
		return nil, resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	billing, _, err := buildXAIBillingSummary(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	return billing, resp, nil
}

func xaiBillingURL() string {
	return "https://cli-chat-proxy.grok.com/v1/billing"
}

func xaiBillingWeeklyURL() string {
	return "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
}

func xaiRequestHeaders(auth *coreauth.Auth) map[string]string {
	headers := map[string]string{
		"Authorization":         "Bearer $TOKEN$",
		"x-xai-token-auth":      "xai-grok-cli",
		"x-grok-client-version": "0.2.91",
		"accept":                "*/*",
		"user-agent":            "grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)",
	}
	if userID := xaiUserID(auth); userID != "" {
		headers["x-userid"] = userID
	}
	return headers
}

func firstInspectionStatus(values ...int) *int {
	return proinspection.FirstNonZeroStatus(values...)
}

func (s *accountInspectionScheduler) antigravityUserAgent() string {
	return misc.AntigravityUserAgent()
}

func (s *accountInspectionScheduler) codexUserAgent() string {
	if s != nil && s.h != nil && s.h.cfg != nil {
		if value := strings.TrimSpace(s.h.cfg.CodexHeaderDefaults.UserAgent); value != "" {
			return value
		}
	}
	return "codex_cli_rs/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9"
}

func (s *accountInspectionScheduler) claudeUserAgent() string {
	if s != nil && s.h != nil && s.h.cfg != nil {
		return strings.TrimSpace(s.h.cfg.ClaudeHeaderDefaults.UserAgent)
	}
	return ""
}

func (s *accountInspectionScheduler) claudeHeaders() map[string]string {
	headers := map[string]string{
		"Authorization":  "Bearer $TOKEN$",
		"Content-Type":   "application/json",
		"anthropic-beta": "oauth-2025-04-20",
	}
	if userAgent := s.claudeUserAgent(); userAgent != "" {
		headers["User-Agent"] = userAgent
	}
	return headers
}

func isAccountErrorStatus(status int) bool {
	return proinspection.IsAccountErrorStatus(status)
}

func isQuotaHTTPStatus(status int) bool {
	return status == http.StatusPaymentRequired || status == http.StatusTooManyRequests
}

func isInspectionAuthRecoveryStatus(status int) bool {
	return (status >= 200 && status < 300) || status == 402 || status == 429
}

func (s *accountInspectionScheduler) syncInspectionAuthError(ctx context.Context, account accountInspectionAccount, code string, message string, status int) {
	if s == nil || s.h == nil || s.h.authManager == nil || account.AuthIndex == "" {
		return
	}
	err := s.h.updateProErrorAuth(ctx, account.AuthIndex, func(auth *coreauth.Auth) {
		auth.Status = coreauth.StatusError
		auth.StatusMessage = message
		auth.Unavailable = true
		syncAuthInspectionLastError(auth, &coreauth.Error{Code: code, Message: message, HTTPStatus: status})
		auth.UpdatedAt = time.Now()
	})
	if err != nil {
		s.appendLog("warning", fmt.Sprintf("%s 认证状态回写失败：%s", account.identity(), err.Error()))
	}
}

func (s *accountInspectionScheduler) clearInspectionAuthError(ctx context.Context, account accountInspectionAccount) {
	if s == nil || s.h == nil || s.h.authManager == nil || account.AuthIndex == "" {
		return
	}
	auth := s.h.authByIndex(account.AuthIndex)
	if auth == nil {
		return
	}
	if !isInspectionAuthErrorCode(authInspectionLastErrorCode(auth)) {
		return
	}
	err := s.h.updateProErrorAuth(ctx, account.AuthIndex, func(auth *coreauth.Auth) {
		if auth.Disabled {
			auth.Status = coreauth.StatusDisabled
		} else {
			auth.Status = coreauth.StatusActive
		}
		auth.StatusMessage = ""
		auth.Unavailable = false
		syncAuthInspectionLastError(auth, nil)
		auth.UpdatedAt = time.Now()
	})
	if err != nil {
		s.appendLog("warning", fmt.Sprintf("%s 认证状态清理失败：%s", account.identity(), err.Error()))
	}
}

func (s *accountInspectionScheduler) syncInspectionAuthStatus(ctx context.Context, account accountInspectionAccount, status int) {
	if isAccountErrorStatus(status) {
		message := fmt.Sprintf("HTTP %d", status)
		s.syncInspectionAuthError(ctx, account, "inspection_http_error", message, status)
		return
	}
	if isInspectionAuthRecoveryStatus(status) {
		s.clearInspectionAuthError(ctx, account)
	}
}

func authErrorDecision(account accountInspectionAccount, status int) accountInspectionDecision {
	return proinspection.AuthErrorDecision(account.Disabled, status)
}

func accountInspectionErrorCode(status *int, fallback string) string {
	return proinspection.ErrorCode(status, fallback)
}

func accountInspectionDecisionErrorCode(provider string, decision accountInspectionDecision, status *int) string {
	return proinspection.DecisionErrorCode(provider, decision, status)
}

func healthyDecision(account accountInspectionAccount) accountInspectionDecision {
	return proinspection.HealthyDecision(account.Disabled)
}

func quotaDecision(account accountInspectionAccount, used *float64, hasQuotaData bool, threshold int) accountInspectionDecision {
	return proinspection.QuotaDecision(account.Disabled, used, hasQuotaData, threshold)
}

func quotaUnavailableDecision(account accountInspectionAccount, reason string, body string) accountInspectionDecision {
	return proinspection.QuotaUnavailableDecision(account.Disabled, reason, inspectionHTTPErrorDetail(body))
}

func codexDecision(account accountInspectionAccount, status int, used *float64, isQuota bool, threshold int) accountInspectionDecision {
	return proinspection.CodexDecision(account.Disabled, status, used, isQuota, threshold)
}

func accountInspectionActionItemFromResult(result accountInspectionResult, action accountInspectionAction) accountInspectionActionItem {
	return proinspection.ActionItemFromResult(result, action)
}

func (s *accountInspectionScheduler) bindActionItemToSnapshot(item accountInspectionActionItem) (accountInspectionActionItem, error) {
	if s == nil {
		return accountInspectionActionItem{}, fmt.Errorf("account inspection scheduler unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, result := range s.status.Results {
		if item.Key != "" {
			if result.Key != item.Key {
				continue
			}
		} else if result.FileName != item.FileName || result.AuthIndex != item.AuthIndex {
			continue
		}
		return accountInspectionActionItemFromResult(result, item.Action), nil
	}
	return accountInspectionActionItem{}, errAccountInspectionResultStale
}

func (s *accountInspectionScheduler) removeInspectionResultLocked(result accountInspectionResult) bool {
	for index, current := range s.status.Results {
		if !sameAccountInspectionResult(current, result) {
			continue
		}
		s.status.Summary = adjustAccountInspectionSummaryForResult(s.status.Summary, current, -1)
		s.healthCounts = adjustAccountInspectionHealthCountsForResult(s.healthCounts, current, -1)
		s.status.Results = append(s.status.Results[:index], s.status.Results[index+1:]...)
		return true
	}
	return false
}

func (s *accountInspectionScheduler) applyManualActionResultLocked(result accountInspectionResult) {
	if result.Key == "" {
		result.Key = accountInspectionKey(result.FileName, result.AuthIndex)
	}
	if result.Executed && result.Action == accountInspectionActionDelete {
		s.removeInspectionResultLocked(result)
		return
	}
	s.updateInspectionResultLocked(result, true, func(current accountInspectionResult) (accountInspectionResult, bool) {
		return proinspection.MergeManualActionResult(current, result)
	})
}

func (s *accountInspectionScheduler) executeManualActions(ctx context.Context, items []accountInspectionActionItem) ([]accountInspectionActionOutcome, error) {
	release, err := s.beginLifecycle()
	if err != nil {
		return nil, err
	}
	defer release()
	s.mu.Lock()
	restoredSnapshot := s.status.RestoredSnapshot
	s.mu.Unlock()
	if restoredSnapshot {
		return nil, errAccountInspectionRestoredSnapshotReadOnly
	}
	boundItems := make([]accountInspectionActionItem, 0, len(items))
	for _, item := range items {
		if item.Action == accountInspectionActionKeep || item.Action == "" {
			continue
		}
		boundItem, err := s.bindActionItemToSnapshot(item)
		if err != nil {
			return nil, err
		}
		boundItems = append(boundItems, boundItem)
	}
	executableItems := dedupeExecutionActionItems(boundItems)
	outcomes := make([]accountInspectionActionOutcome, len(executableItems))
	executedResults := make([]accountInspectionResult, len(executableItems))
	runAccountInspectionWorkers(len(executableItems), accountInspectionMaxDeleteWorkers, nil, func(index int) bool {
		item := executableItems[index]
		result := item.ToResult()
		action := item.Action
		outcome := accountInspectionActionOutcome{Action: action, FileName: item.FileName, DisplayName: item.DisplayName, Email: item.Email, Name: item.Name, Provider: item.Provider, AuthIndex: item.AuthIndex}
		if err := s.executeAction(ctx, result, action); err != nil {
			outcome.Error = err.Error()
			result.ExecuteError = err.Error()
			s.appendLog("error", fmt.Sprintf("%s -> %s 执行失败：%s", resultIdentity(result), action, err.Error()))
		} else {
			outcome.Success = true
			result.Executed = true
			result.ExecuteError = ""
			if action == accountInspectionActionDisable {
				result.Disabled = true
			}
			if action == accountInspectionActionEnable {
				result.Disabled = false
			}
			s.appendLog("success", fmt.Sprintf("%s %s 成功", resultIdentity(result), action))
		}
		outcomes[index] = outcome
		executedResults[index] = result
		return true
	})

	s.mu.Lock()
	for _, result := range executedResults {
		if result.FileName == "" {
			continue
		}
		s.applyManualActionResultLocked(result)
	}
	s.status.Results = sortAccountInspectionResults(s.status.Results)
	saveErr := s.saveResultSnapshotLocked()
	broadcast := s.statusBroadcastLocked()
	s.mu.Unlock()
	broadcast.send()
	if saveErr != nil {
		return outcomes, fmt.Errorf("failed to save account inspection snapshot: %w", saveErr)
	}
	return outcomes, nil
}

func dedupeExecutionActionItems(items []accountInspectionActionItem) []accountInspectionActionItem {
	return proinspection.DedupeActionItems(items)
}

func summarizeManualActionOutcomes(outcomes []accountInspectionActionOutcome) proinspection.ActionOutcomeSummary {
	return proinspection.SummarizeActionOutcomes(outcomes)
}

func (s *accountInspectionScheduler) applyAutomaticActions(ctx context.Context, results []accountInspectionResult, settings accountInspectionSettings) {
	workers := settings.DeleteWorkers
	if workers <= 0 {
		workers = settings.Workers
	}
	deletedFiles := make(map[string]struct{})
	var mu sync.Mutex
	runAccountInspectionWorkers(len(results), workers, nil, func(index int) bool {
		action := autoActionForResult(results[index], settings)
		if action == "" {
			s.clearAutoActionConfirmation(results[index])
			return true
		}
		confirmed, count, required := s.confirmAutoAction(results[index], action, settings.AutoExecuteConfirmations)
		if !confirmed {
			if results[index].ActionReason != "" {
				results[index].ActionReason += fmt.Sprintf("；等待连续确认 %d/%d 后自动执行", count, required)
			}
			s.appendLog("info", fmt.Sprintf("%s -> %s 等待连续确认 %d/%d", resultIdentity(results[index]), action, count, required))
			return true
		}
		if action == accountInspectionActionDelete {
			mu.Lock()
			if _, ok := deletedFiles[results[index].FileName]; ok {
				results[index].ExecuteError = "auth file already deleted in this inspection run"
				mu.Unlock()
				return true
			}
			deletedFiles[results[index].FileName] = struct{}{}
			mu.Unlock()
		}
		err := s.executeAction(ctx, results[index], action)
		mu.Lock()
		if err != nil {
			results[index].ExecuteError = err.Error()
			s.appendLog("error", fmt.Sprintf("%s -> %s 执行失败：%s", resultIdentity(results[index]), action, err.Error()))
		} else {
			results[index].Executed = true
			results[index].Action = action
			s.clearAutoActionConfirmation(results[index])
			if action == accountInspectionActionDisable {
				results[index].Disabled = true
			}
			if action == accountInspectionActionEnable {
				results[index].Disabled = false
			}
			s.appendLog("success", fmt.Sprintf("%s %s 成功", resultIdentity(results[index]), action))
		}
		mu.Unlock()
		return true
	})
}

func (s *accountInspectionScheduler) confirmAutoAction(result accountInspectionResult, action accountInspectionAction, required int) (bool, int, int) {
	key := autoActionConfirmationKey(result, action)
	if s == nil {
		return true, 1, required
	}
	s.mu.Lock()
	if s.autoActionConfirmations == nil {
		s.autoActionConfirmations = proinspection.NewConfirmationCounter()
	}
	confirmations := s.autoActionConfirmations
	s.mu.Unlock()
	return confirmations.Confirm(key, required)
}

func (s *accountInspectionScheduler) clearAutoActionConfirmation(result accountInspectionResult) {
	keyPrefix := result.Key
	if keyPrefix == "" {
		keyPrefix = result.FileName + ":" + result.AuthIndex
	}
	if keyPrefix == "" {
		return
	}
	if s != nil {
		s.mu.Lock()
		confirmations := s.autoActionConfirmations
		s.mu.Unlock()
		if confirmations != nil {
			confirmations.ClearPrefix(keyPrefix + "|")
		}
	}
}

func autoActionConfirmationKey(result accountInspectionResult, action accountInspectionAction) string {
	return proinspection.AutoActionConfirmationKey(result, action)
}

func accountInspectionAutoActionForError(result accountInspectionResult, action accountInspectionAction) accountInspectionAction {
	return proinspection.AutoActionForError(result, action)
}

func isAccountInspectionAccountInvalidResult(result accountInspectionResult) bool {
	return proinspection.IsAccountInvalidResult(result)
}

func isAccountInspectionRequestErrorResult(result accountInspectionResult) bool {
	return proinspection.IsRequestErrorResult(result)
}

func autoActionForResult(result accountInspectionResult, settings accountInspectionSettings) accountInspectionAction {
	return proinspection.AutoActionForResult(result, settings)
}

func (s *accountInspectionScheduler) executeAction(ctx context.Context, result accountInspectionResult, action accountInspectionAction) error {
	if s.h == nil || s.h.authManager == nil {
		return fmt.Errorf("core auth manager unavailable")
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	auth, err := s.actionAuthForResult(result)
	if err != nil {
		return err
	}
	switch action {
	case accountInspectionActionDisable, accountInspectionActionEnable:
		return s.h.updateProAuth(ctx, result.AuthIndex, func(auth *coreauth.Auth) {
			setProAuthDisabledState(auth, action == accountInspectionActionDisable)
		})
	case accountInspectionActionDelete:
		if s.pluginVirtualSourceAuthCount(auth) > 1 {
			return errAccountInspectionSharedSourceDelete
		}
		_, _, err := s.h.deleteAuthFileByName(ctx, accountFromAuth(auth).FileName)
		return err
	default:
		return fmt.Errorf("unsupported action %s", action)
	}
}

func (s *accountInspectionScheduler) actionAuthForResult(result accountInspectionResult) (*coreauth.Auth, error) {
	if s == nil || s.h == nil || s.h.authManager == nil || strings.TrimSpace(result.AuthIndex) == "" {
		return nil, errAccountInspectionResultStale
	}
	auth := s.h.authByIndex(result.AuthIndex)
	if auth == nil {
		return nil, errAccountInspectionResultStale
	}
	account := accountFromAuth(auth)
	if result.Key != "" && result.Key != account.Key {
		return nil, errAccountInspectionResultStale
	}
	if result.FileName != "" && result.FileName != account.FileName {
		return nil, errAccountInspectionResultStale
	}
	if result.Provider != "" && !strings.EqualFold(strings.TrimSpace(result.Provider), account.Provider) {
		return nil, errAccountInspectionResultStale
	}
	return auth, nil
}

func (s *accountInspectionScheduler) pluginVirtualSourceAuthCount(auth *coreauth.Auth) int {
	if s == nil || s.h == nil || s.h.authManager == nil || auth == nil || !coreauth.IsPluginVirtualAuth(auth) {
		return 0
	}
	sourcePath := pluginVirtualSourcePath(auth)
	if sourcePath == "" {
		return 0
	}
	count := 0
	for _, candidate := range s.h.authManager.List() {
		if candidate != nil && coreauth.IsPluginVirtualAuth(candidate) && sameAuthSourcePath(pluginVirtualSourcePath(candidate), sourcePath) {
			count++
		}
	}
	return count
}

func summarizeAccountInspection(totalFiles int, probeSetCount int, accounts []accountInspectionAccount, results []accountInspectionResult) accountInspectionSummary {
	disabledCount := 0
	for _, account := range accounts {
		if account.Disabled {
			disabledCount++
		}
	}
	return proinspection.SummarizeResults(totalFiles, probeSetCount, disabledCount, len(accounts)-disabledCount, results)
}

func sortAccountInspectionResults(results []accountInspectionResult) []accountInspectionResult {
	return proinspection.SortResults(results)
}

func adjustAccountInspectionSummaryForResult(summary accountInspectionSummary, result accountInspectionResult, delta int) accountInspectionSummary {
	return proinspection.AdjustSummaryForResult(summary, result, delta)
}

func resultIdentity(result accountInspectionResult) string {
	return proinspection.ResultIdentity(result)
}

func quotaSuccessState(values map[string]any) map[string]any {
	return proquota.SuccessCacheState(accountInspectionQuotaParserVersion, values)
}

func jsonShapeHash(body string) string {
	return proquota.JSONShapeHash(body)
}

func jsonShapeHashForBodies(bodies map[string]string) string {
	return proquota.JSONShapeHashForBodies(bodies)
}

func (s *accountInspectionScheduler) persistQuotaState(ctx context.Context, account accountInspectionAccount, state map[string]any) {
	if err := persistQuotaState(ctx, account, state); err != nil {
		s.appendLog("warning", fmt.Sprintf("%s 配额缓存写入失败：%s", account.identity(), err.Error()))
		return
	}
	if err := s.cleanupLegacyQuotaCacheFromAuth(ctx, account); err != nil {
		s.appendLog("warning", fmt.Sprintf("%s 旧认证文件配额缓存清理失败：%s", account.identity(), err.Error()))
	}
}

func (s *accountInspectionScheduler) cleanupLegacyQuotaCacheFromAuth(ctx context.Context, account accountInspectionAccount) error {
	if s == nil || s.h == nil || s.h.authManager == nil || account.AuthIndex == "" {
		return nil
	}
	auth := s.h.authByIndex(account.AuthIndex)
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	if _, exists := auth.Metadata["quota_cache"]; !exists {
		return nil
	}
	return s.h.updateProAuth(ctx, account.AuthIndex, func(auth *coreauth.Auth) {
		if auth.Metadata == nil {
			return
		}
		delete(auth.Metadata, "quota_cache")
		auth.UpdatedAt = time.Now()
	})
}

func (s *accountInspectionScheduler) cleanupLegacyQuotaCaches(ctx context.Context) {
	if s == nil || s.h == nil || s.h.authManager == nil {
		return
	}
	for _, auth := range s.h.authManager.List() {
		if auth == nil || auth.Metadata == nil {
			continue
		}
		if _, exists := auth.Metadata["quota_cache"]; !exists {
			continue
		}
		account := accountFromAuth(auth)
		if err := s.cleanupLegacyQuotaCacheFromAuth(ctx, account); err != nil {
			s.appendLog("warning", fmt.Sprintf("%s 启动清理旧认证文件配额缓存失败：%s", account.identity(), err.Error()))
		}
	}
}

func persistQuotaState(ctx context.Context, account accountInspectionAccount, state map[string]any) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	observedAt := now
	if cachedAt, ok := intFromAny(state["cachedAt"]); ok && cachedAt > 0 {
		observedAt = int64(cachedAt)
	}
	version := 1
	if schemaVersion, ok := intFromAny(state["schemaVersion"]); ok && schemaVersion > 0 {
		version = schemaVersion
	}
	fingerprintSource := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(account.Provider)),
		strings.ToLower(strings.TrimSpace(account.FileName)),
		strings.ToLower(strings.TrimSpace(account.Email)),
		strings.ToLower(strings.TrimSpace(account.Name)),
	}, "|")
	fingerprint := sha256.Sum256([]byte(fingerprintSource))
	entry := embeddedusage.QuotaCacheEntry{
		ID:                  account.Provider + ":" + account.FileName,
		Provider:            account.Provider,
		FileName:            account.FileName,
		AuthIndex:           account.AuthIndex,
		IdentityFingerprint: hex.EncodeToString(fingerprint[:]),
		Data:                raw,
		CachedAt:            observedAt,
		ObservedAt:          observedAt,
		AccessedAt:          now,
		Version:             version,
	}
	if strings.EqualFold(strings.TrimSpace(account.Provider), "xai") {
		return embeddedusage.MergeXAIQuotaCache(ctx, entry)
	}
	return embeddedusage.SetQuotaCache(ctx, entry)
}

func buildAntigravityGroups(body string) ([]map[string]any, error) {
	return proinspection.BuildAntigravityGroups(body)
}

func buildAntigravitySubscription(payload map[string]any) map[string]any {
	return proinspection.BuildAntigravitySubscription(payload)
}

func parseAntigravityQuotaPayload(body string) (map[string]any, error) {
	return proinspection.ParseAntigravityQuotaPayload(body)
}

func antigravityUsedPercent(groups []map[string]any, mode accountInspectionAntigravityQuotaMode) *float64 {
	return proinspection.AntigravityUsedPercent(groups, mode)
}

func antigravityGroupUsedPercent(group map[string]any) *float64 {
	return proinspection.AntigravityGroupUsedPercent(group)
}

func buildClaudeWindows(body string) ([]map[string]any, any, error) {
	return proinspection.BuildClaudeWindows(body)
}

func resolveClaudePlan(body string) string {
	return proinspection.ResolveClaudePlan(body)
}

func buildCodexWindows(body string) (map[string]any, []map[string]any, *float64) {
	return proinspection.BuildCodexWindows(body)
}

func buildKimiRows(body string) ([]map[string]any, *float64, error) {
	return proinspection.BuildKimiRows(body)
}

func buildXAIBillingSummary(body string) (map[string]any, *float64, error) {
	return proquota.BuildXAIBillingSummary(body)
}

func mergeXAIBillingSummaries(primary map[string]any, fallback map[string]any) map[string]any {
	return proquota.MergeXAIBillingSummaries(primary, fallback)
}

func xaiSummaryUsedPercent(summary map[string]any) *float64 {
	return proquota.XAISummaryUsedPercent(summary)
}

func emptyXAIBillingSummary() map[string]any {
	return proquota.EmptyXAIBillingSummary()
}

func xaiPlanTypeFromBillingBody(status int, body string) (string, bool) {
	return proquota.XAIPlanTypeFromBillingBody(status, body)
}

func mergeCachedXAIFreeQuota(ctx context.Context, account accountInspectionAccount, billing map[string]any) map[string]any {
	state, ok, err := embeddedusage.GetXAIQuotaState(ctx, account.FileName)
	if err != nil || !ok {
		return billing
	}
	cachedBilling := firstMap(state, "billing")
	freeQuota := firstMap(cachedBilling, "freeQuota", "free_quota")
	if freeQuota == nil {
		return billing
	}
	if billing == nil {
		billing = emptyXAIBillingSummary()
	}
	billing["freeQuota"] = freeQuota
	return billing
}

func observeAccountXAIQuota(ctx context.Context, account accountInspectionAccount, model string, result accountInspectionHTTPResult) {
	_ = embeddedusage.ObserveXAIQuotaResponse(ctx, embeddedusage.XAIQuotaObservation{
		FileName:   account.FileName,
		AuthIndex:  account.AuthIndex,
		Email:      account.Email,
		Label:      firstNonEmptyStringValue(account.Name, account.DisplayName),
		Model:      model,
		Status:     result.StatusCode,
		Header:     result.Header,
		Body:       []byte(result.Body),
		ObservedAt: time.Now(),
	})
}

func floatPtrAny(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func intFromAny(value any) (int, bool) {
	parsed, ok := floatFromAny(value)
	if !ok {
		return 0, false
	}
	return int(parsed), true
}

func maxUsedPercentFromWindows(windows []map[string]any) *float64 {
	return proinspection.MaxUsedPercentFromWindows(windows)
}

func maxFloatPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	maxValue := values[0]
	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return &maxValue
}

func antigravityProjectID(auth *coreauth.Auth) string {
	for _, source := range []map[string]any{auth.Metadata, nestedMap(auth.Metadata, "installed"), nestedMap(auth.Metadata, "web")} {
		if source == nil {
			continue
		}
		if value := firstNonEmptyStringValue(stringFromAny(source["project_id"]), stringFromAny(source["projectId"])); value != "" {
			return value
		}
	}
	return "bamboo-precept-lgxtn"
}

func codexAccountID(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, source := range []map[string]any{auth.Metadata, stringMapToAnyMap(auth.Attributes)} {
		if value := codexAccountIDFromMap(source); value != "" {
			return value
		}
	}
	return ""
}

func codexAccountIDFromMap(source map[string]any) string {
	if source == nil {
		return ""
	}
	for _, key := range []string{"chatgpt_account_id", "chatgptAccountId", "account_id", "accountId"} {
		if value := stringFromAny(source[key]); value != "" {
			return value
		}
	}
	return idTokenClaim(source["id_token"], "chatgpt_account_id", "chatgptAccountId", "account_id", "accountId")
}

func xaiUserID(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, source := range []map[string]any{auth.Metadata, stringMapToAnyMap(auth.Attributes)} {
		if value := xaiUserIDFromMap(source); value != "" {
			return value
		}
	}
	return ""
}

func xaiUserIDFromMap(source map[string]any) string {
	if source == nil {
		return ""
	}
	for _, key := range []string{"x_user_id", "xUserId", "user_id", "userId", "subject", "sub", "id"} {
		if value := stringFromAny(source[key]); value != "" {
			return value
		}
	}
	return idTokenStringClaim(source["id_token"], "sub", "id", "user_id", "userId")
}

func idTokenStringClaim(raw any, keys ...string) string {
	if mapped, ok := raw.(map[string]any); ok {
		for _, key := range keys {
			if value := stringFromAny(mapped[key]); value != "" {
				return value
			}
		}
		return ""
	}
	token := stringFromAny(raw)
	if token == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(token), &parsed); err == nil {
		for _, key := range keys {
			if value := stringFromAny(parsed[key]); value != "" {
				return value
			}
		}
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	for _, key := range keys {
		if value := stringFromAny(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringMapToAnyMap(values map[string]string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func codexPlanType(auth *coreauth.Auth, payload map[string]any) any {
	if value := firstNonEmptyStringValue(stringFromAny(payload["plan_type"]), stringFromAny(payload["planType"])); value != "" {
		return value
	}
	for _, raw := range []any{auth.Metadata["plan_type"], auth.Metadata["planType"], auth.Attributes["plan_type"], auth.Attributes["planType"]} {
		if value := stringFromAny(raw); value != "" {
			return value
		}
	}
	return nil
}

func codexQuotaStateValues(auth *coreauth.Auth, payload map[string]any, windows []map[string]any, rawBody string) map[string]any {
	values := map[string]any{
		"windows":      windows,
		"planType":     codexPlanType(auth, payload),
		"rawShapeHash": jsonShapeHash(rawBody),
	}
	values["subscriptionActiveUntil"] = codexSubscriptionActiveUntil(auth)
	values["rateLimitResetCreditsAvailableCount"] = codexRateLimitResetCreditsAvailableCount(payload)
	return values
}

func codexSubscriptionActiveUntil(auth *coreauth.Auth) any {
	if auth == nil {
		return nil
	}
	for _, source := range []map[string]any{auth.Metadata, stringMapToAnyMap(auth.Attributes)} {
		if value := codexSubscriptionActiveUntilFromMap(source); value != nil {
			return value
		}
	}
	return nil
}

func codexSubscriptionActiveUntilFromMap(source map[string]any) any {
	if source == nil {
		return nil
	}
	for _, key := range []string{"chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil", "subscription_active_until", "subscriptionActiveUntil"} {
		if value := dateLikeValue(source[key]); value != nil {
			return value
		}
	}
	for _, rawSubscription := range []any{source["subscription"], source["Subscription"]} {
		if subscription, ok := rawSubscription.(map[string]any); ok {
			for _, key := range []string{"active_until", "activeUntil"} {
				if value := dateLikeValue(subscription[key]); value != nil {
					return value
				}
			}
		}
	}
	if value := idTokenClaimAny(source["id_token"], "chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil", "subscription_active_until", "subscriptionActiveUntil"); value != nil {
		return value
	}
	return nil
}

func codexRateLimitResetCreditsAvailableCount(payload map[string]any) any {
	if payload == nil {
		return nil
	}
	resetCredits, _ := firstAny(payload, "rate_limit_reset_credits", "rateLimitResetCredits").(map[string]any)
	if resetCredits == nil {
		return nil
	}
	if value, ok := floatFromAny(firstAny(resetCredits, "available_count", "availableCount")); ok {
		return value
	}
	return nil
}

func dateLikeValue(value any) any {
	if number, ok := floatFromAny(value); ok {
		if number == 0 {
			return nil
		}
		return value
	}
	if text := stringFromAny(value); text != "" && text != "0" {
		return text
	}
	return nil
}

func idTokenClaim(raw any, keys ...string) string {
	value := idTokenClaimAny(raw, keys...)
	if text := stringFromAny(value); text != "" {
		return text
	}
	return ""
}

func idTokenClaimAny(raw any, keys ...string) any {
	switch value := raw.(type) {
	case map[string]any:
		for _, key := range keys {
			if claim := dateLikeValue(value[key]); claim != nil {
				return claim
			}
		}
		return nil
	}
	token := stringFromAny(raw)
	if token == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(token), &parsed); err == nil {
		for _, key := range keys {
			if value := dateLikeValue(parsed[key]); value != nil {
				return value
			}
		}
		return nil
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil
	}
	for _, key := range keys {
		if value := dateLikeValue(data[key]); value != nil {
			return value
		}
	}
	return nil
}

func accountInspectionAuthEmail(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if value := firstNonEmptyAuthValue(auth, "email"); value != "" {
		return value
	}
	return idTokenClaim(auth.Metadata["id_token"], "email")
}

func firstNonEmptyAuthValue(auth *coreauth.Auth, keys ...string) string {
	if auth == nil {
		return ""
	}
	for _, key := range keys {
		if value := stringFromAny(auth.Metadata[key]); value != "" {
			return value
		}
		if value := strings.TrimSpace(auth.Attributes[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstAny(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func firstMap(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func nestedMap(data map[string]any, key string) map[string]any {
	if data == nil {
		return nil
	}
	value, _ := data[key].(map[string]any)
	return value
}

func nestedString(data map[string]any, key string, child string) string {
	if child == "" {
		return stringFromAny(data[key])
	}
	return stringFromAny(nestedMap(data, key)[child])
}

func firstNonEmptyStringValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func anySlice(value any) []any {
	switch items := value.(type) {
	case []any:
		return items
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	case []string:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func formatResetTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "-"
	}
	return parsed.Local().Format("01/02, 15:04")
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func boolValue(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(v))
		if trimmed == "true" || trimmed == "1" || trimmed == "yes" || trimmed == "y" || trimmed == "on" {
			return true, true
		}
		if trimmed == "false" || trimmed == "0" || trimmed == "no" || trimmed == "n" || trimmed == "off" {
			return false, true
		}
	case float64:
		return v != 0, true
	case int:
		return v != 0, true
	}
	return false, false
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

func normalizeFraction(value float64) float64 {
	if value > 1 && value <= 100 {
		value = value / 100
	}
	return math.Max(0, math.Min(1, value))
}

func intPtr(value int) *int {
	return &value
}

func firstNonNilError(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}

func emptyStringAsNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func escapeJSONString(value string) string {
	raw, _ := json.Marshal(value)
	return strings.Trim(string(raw), "\"")
}

func (h *Handler) RegisterAccountInspectionRoutes(group *gin.RouterGroup) {
	group.GET("/account-inspection/logs", h.StreamAccountInspectionLogs)
	group.GET("/account-inspection/schedule", h.GetAccountInspectionSchedule)
	group.PUT("/account-inspection/schedule", h.PutAccountInspectionSchedule)
	group.PATCH("/account-inspection/schedule", h.PutAccountInspectionSchedule)
	group.GET("/account-inspection/status", h.GetAccountInspectionStatus)
	group.POST("/account-inspection/run", h.RunAccountInspection)
	group.POST("/account-inspection/inspect-one", h.InspectOneAccount)
	group.POST("/account-inspection/refresh-token", h.RefreshAccountInspectionToken)
	group.POST("/account-inspection/pause", h.PauseAccountInspection)
	group.POST("/account-inspection/resume", h.ResumeAccountInspection)
	group.POST("/account-inspection/stop", h.StopAccountInspection)
	group.POST("/account-inspection/actions", h.ExecuteAccountInspectionActions)
}

func (h *Handler) GetAccountInspectionSchedule(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}

func (h *Handler) PutAccountInspectionSchedule(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	var schedule accountInspectionSchedule
	if err := c.ShouldBindJSON(&schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := scheduler.update(schedule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}

func (h *Handler) RunAccountInspection(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	if err := scheduler.startRun(true); err != nil {
		c.JSON(http.StatusConflict, scheduler.snapshotForRequest(c))
		return
	}
	c.JSON(http.StatusAccepted, scheduler.snapshotForRequest(c))
}

func (h *Handler) InspectOneAccount(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	var request accountInspectionOneRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	result, err := scheduler.inspectOne(c.Request.Context(), request.Item)
	snapshot := scheduler.snapshotForRequest(c)
	if err != nil {
		statusCode := http.StatusOK
		if errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) || errors.Is(err, errAccountInspectionResultStale) {
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{"error": err.Error(), "result": result, "schedule": snapshot["schedule"], "status": snapshot["status"]})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result, "schedule": snapshot["schedule"], "status": snapshot["status"]})
}

func (h *Handler) RefreshAccountInspectionToken(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	var request accountInspectionRefreshTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	result, err := scheduler.refreshTokenNow(c.Request.Context(), request.Item)
	scheduler.mu.Lock()
	var saveErr error
	if result.Key != "" {
		scheduler.mergeTokenRefreshResultLocked(result)
		scheduler.status.Results = sortAccountInspectionResults(scheduler.status.Results)
		saveErr = scheduler.saveResultSnapshotLocked()
	}
	broadcast := scheduler.statusBroadcastLocked()
	scheduler.mu.Unlock()
	broadcast.send()
	err = firstNonNilError(err, saveErr)
	snapshot := scheduler.snapshotForRequest(c)
	if err != nil {
		statusCode := http.StatusOK
		if errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) || errors.Is(err, errAccountInspectionResultStale) {
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{"error": err.Error(), "result": result, "schedule": snapshot["schedule"], "status": snapshot["status"]})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result, "schedule": snapshot["schedule"], "status": snapshot["status"]})
}

func (h *Handler) PauseAccountInspection(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	scheduler.pauseRun()
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}

func (h *Handler) ResumeAccountInspection(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	scheduler.resumeRun()
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}

func (h *Handler) StopAccountInspection(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	scheduler.stopRun()
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}

func (h *Handler) ExecuteAccountInspectionActions(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	var request accountInspectionActionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	outcomes, err := scheduler.executeManualActions(c.Request.Context(), request.Items)
	snapshot := scheduler.snapshotForRequest(c)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) || errors.Is(err, errAccountInspectionResultStale) {
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{
			"error":    err.Error(),
			"outcomes": outcomes,
			"summary":  summarizeManualActionOutcomes(outcomes),
			"schedule": snapshot["schedule"],
			"status":   snapshot["status"],
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"outcomes": outcomes,
		"summary":  summarizeManualActionOutcomes(outcomes),
		"schedule": snapshot["schedule"],
		"status":   snapshot["status"],
	})
}

func (h *Handler) StreamAccountInspectionLogs(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	scheduler.streamLogs(c)
}

func (h *Handler) GetAccountInspectionStatus(c *gin.Context) {
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account inspection scheduler unavailable"})
		return
	}
	c.JSON(http.StatusOK, scheduler.snapshotForRequest(c))
}
