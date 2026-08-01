package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
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
			Settings:        proinspection.DefaultSettings(),
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

func normalizeAccountInspectionSchedule(input accountInspectionSchedule) accountInspectionSchedule {
	return proinspection.NormalizeSchedule(input, time.Now())
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
		State:          proinspection.NormalizeSnapshotState(s.status.State),
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
		s.status.Results = proinspection.SortResults(s.status.Results)
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
	release, err := s.beginLifecycle()
	if err != nil {
		return accountInspectionResult{}, err
	}
	defer release()
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

func (s *accountInspectionScheduler) updateInspectionResultLocked(result accountInspectionResult, appendMissing bool, update func(accountInspectionResult) (accountInspectionResult, bool)) bool {
	if result.Key == "" {
		return false
	}

	for index, current := range s.status.Results {
		if proinspection.SameResult(current, result) {
			merged, updateSummary := update(current)
			if updateSummary {
				s.status.Summary = proinspection.AdjustSummaryForResult(s.status.Summary, current, -1)
				s.status.Summary = proinspection.AdjustSummaryForResult(s.status.Summary, merged, 1)
			}
			s.healthCounts = proinspection.AdjustHealthCountsForResult(s.healthCounts, current, -1)
			s.healthCounts = proinspection.AdjustHealthCountsForResult(s.healthCounts, merged, 1)
			s.status.Results[index] = merged
			return true
		}
	}

	if !appendMissing {
		return false
	}
	s.status.Summary = proinspection.AdjustSummaryForResult(s.status.Summary, result, 1)
	s.healthCounts = proinspection.AdjustHealthCountsForResult(s.healthCounts, result, 1)
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
	s.healthCounts = proinspection.ResultHealthCounts(results)
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
