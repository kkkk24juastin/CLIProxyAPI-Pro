package inspection

import (
	"strings"
	"time"
)

const (
	ProviderAll        = "all"
	DefaultIntervalMin = 360
	DefaultTimeoutMS   = 15000
	MinTimeoutMS       = 3000
	MaxTimeoutMS       = 30000
	MaxWorkers         = 8
	MaxDeleteWorkers   = 4
	MaxRetries         = 1
)

var supportedProviders = []string{"antigravity", "claude", "codex", "gemini-cli", "kimi", "xai"}

type AntigravityQuotaMode string

const (
	AntigravityQuotaModeMaxUsed   AntigravityQuotaMode = "max-used"
	AntigravityQuotaModeClaudeGPT AntigravityQuotaMode = "claude-gpt"
)

type Settings struct {
	TargetType                      string               `json:"targetType"`
	Workers                         int                  `json:"workers"`
	DeleteWorkers                   int                  `json:"deleteWorkers"`
	Timeout                         int                  `json:"timeout"`
	Retries                         int                  `json:"retries"`
	UsedPercentThreshold            int                  `json:"usedPercentThreshold"`
	SampleSize                      int                  `json:"sampleSize"`
	AntigravityDeepProbeEnabled     bool                 `json:"antigravityDeepProbeEnabled"`
	AntigravityDeepProbeModel       string               `json:"antigravityDeepProbeModel"`
	AntigravityQuotaMode            AntigravityQuotaMode `json:"antigravityQuotaMode"`
	XAIDeepProbeEnabled             bool                 `json:"xaiDeepProbeEnabled"`
	XAIDeepProbeModel               string               `json:"xaiDeepProbeModel"`
	AutoExecuteQuotaLimitDisable    bool                 `json:"autoExecuteQuotaLimitDisable"`
	AutoExecuteQuotaRecoveryEnable  bool                 `json:"autoExecuteQuotaRecoveryEnable"`
	AutoExecuteAccountInvalidAction Action               `json:"autoExecuteAccountInvalidAction"`
	AutoExecuteRequestErrorAction   Action               `json:"autoExecuteRequestErrorAction"`
	AutoExecuteConfirmations        int                  `json:"autoExecuteConfirmations,omitempty"`
	EnabledOnly                     bool                 `json:"-"`
	QuotaOnly                       bool                 `json:"-"`
}

type Schedule struct {
	Enabled         bool     `json:"enabled"`
	IntervalMinutes int      `json:"intervalMinutes"`
	NextRunAt       int64    `json:"nextRunAt"`
	Settings        Settings `json:"settings"`
}

func SupportedProviderSet() map[string]struct{} {
	result := make(map[string]struct{}, len(supportedProviders))
	for _, provider := range supportedProviders {
		result[provider] = struct{}{}
	}
	return result
}

func DefaultSettings() Settings {
	return Settings{
		TargetType:                      ProviderAll,
		Workers:                         4,
		DeleteWorkers:                   4,
		Timeout:                         DefaultTimeoutMS,
		UsedPercentThreshold:            100,
		AntigravityDeepProbeModel:       "claude-sonnet-4-6",
		AntigravityQuotaMode:            AntigravityQuotaModeClaudeGPT,
		XAIDeepProbeModel:               "grok-4.5",
		AutoExecuteAccountInvalidAction: ActionNone,
		AutoExecuteRequestErrorAction:   ActionNone,
		AutoExecuteConfirmations:        1,
	}
}

func NormalizeSchedule(input Schedule, now time.Time) Schedule {
	defaults := DefaultSettings()
	settings := input.Settings
	settings.TargetType = strings.ToLower(strings.TrimSpace(settings.TargetType))
	if settings.TargetType == "" || (!IsSupportedProvider(settings.TargetType) && settings.TargetType != ProviderAll) {
		settings.TargetType = defaults.TargetType
	}
	if settings.Workers <= 0 {
		settings.Workers = defaults.Workers
	}
	if settings.Workers > MaxWorkers {
		settings.Workers = MaxWorkers
	}
	if settings.DeleteWorkers <= 0 {
		settings.DeleteWorkers = settings.Workers
	}
	if settings.DeleteWorkers > MaxDeleteWorkers {
		settings.DeleteWorkers = MaxDeleteWorkers
	}
	if settings.Timeout <= 0 {
		settings.Timeout = defaults.Timeout
	}
	if settings.Timeout < MinTimeoutMS {
		settings.Timeout = MinTimeoutMS
	}
	if settings.Timeout > MaxTimeoutMS {
		settings.Timeout = MaxTimeoutMS
	}
	if settings.Retries < 0 {
		settings.Retries = 0
	}
	if settings.Retries > MaxRetries {
		settings.Retries = MaxRetries
	}
	if settings.UsedPercentThreshold < 0 {
		settings.UsedPercentThreshold = 0
	}
	if settings.UsedPercentThreshold > 100 {
		settings.UsedPercentThreshold = 100
	}
	if settings.SampleSize < 0 {
		settings.SampleSize = 0
	}
	if settings.AutoExecuteConfirmations <= 0 {
		settings.AutoExecuteConfirmations = defaults.AutoExecuteConfirmations
	}
	if settings.AutoExecuteConfirmations > 5 {
		settings.AutoExecuteConfirmations = 5
	}
	settings.AntigravityDeepProbeModel = strings.TrimSpace(settings.AntigravityDeepProbeModel)
	if settings.AntigravityDeepProbeModel == "" {
		settings.AntigravityDeepProbeModel = defaults.AntigravityDeepProbeModel
	}
	settings.AntigravityQuotaMode = AntigravityQuotaMode(strings.ToLower(strings.TrimSpace(string(settings.AntigravityQuotaMode))))
	if settings.AntigravityQuotaMode != AntigravityQuotaModeMaxUsed && settings.AntigravityQuotaMode != AntigravityQuotaModeClaudeGPT {
		settings.AntigravityQuotaMode = defaults.AntigravityQuotaMode
	}
	settings.XAIDeepProbeModel = strings.TrimSpace(settings.XAIDeepProbeModel)
	if settings.XAIDeepProbeModel == "" {
		settings.XAIDeepProbeModel = defaults.XAIDeepProbeModel
	}
	settings.AutoExecuteAccountInvalidAction = NormalizeAutoAction(settings.AutoExecuteAccountInvalidAction)
	settings.AutoExecuteRequestErrorAction = NormalizeAutoAction(settings.AutoExecuteRequestErrorAction)
	input.Settings = settings
	if input.IntervalMinutes <= 0 {
		input.IntervalMinutes = DefaultIntervalMin
	}
	if input.Enabled && input.NextRunAt <= 0 {
		input.NextRunAt = now.Add(time.Duration(input.IntervalMinutes) * time.Minute).UnixMilli()
	}
	if !input.Enabled {
		input.NextRunAt = 0
	}
	return input
}

func NormalizeAutoAction(action Action) Action {
	action = Action(strings.ToLower(strings.TrimSpace(string(action))))
	if action == ActionDisable || action == ActionDelete {
		return action
	}
	return ActionNone
}

func IsSupportedProvider(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, supported := range supportedProviders {
		if provider == supported {
			return true
		}
	}
	return false
}
