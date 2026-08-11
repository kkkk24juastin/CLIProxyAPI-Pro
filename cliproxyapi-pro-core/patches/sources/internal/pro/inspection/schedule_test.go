package inspection

import (
	"testing"
	"time"
)

func TestNormalizeScheduleOwnsDefaultsAndBounds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	got := NormalizeSchedule(Schedule{
		Enabled: true,
		Settings: Settings{
			TargetType: "unknown", Workers: 99, ProviderWorkers: 99, DeleteWorkers: 99,
			Timeout: 1, Retries: 99, UsedPercentThreshold: 101,
		},
	}, now)
	if got.Settings.TargetType != ProviderAll || got.Settings.Workers != MaxWorkers || got.Settings.ProviderWorkers != MaxProviderWorkers || got.Settings.DeleteWorkers != MaxDeleteWorkers {
		t.Fatalf("normalized workers/providers = %+v", got.Settings)
	}
	if got.Settings.Timeout != MinTimeoutMS || got.Settings.Retries != MaxRetries || got.Settings.UsedPercentThreshold != 100 {
		t.Fatalf("normalized limits = %+v", got.Settings)
	}
	if got.NextRunAt != now.Add(DefaultIntervalMin*time.Minute).UnixMilli() {
		t.Fatalf("next run = %d", got.NextRunAt)
	}
}

func TestNormalizeScheduleDefaultsProviderWorkersForLegacySettings(t *testing.T) {
	got := NormalizeSchedule(Schedule{Settings: Settings{Workers: 6}}, time.Now())
	if got.Settings.Workers != 6 || got.Settings.ProviderWorkers != DefaultSettings().ProviderWorkers {
		t.Fatalf("normalized legacy workers = %+v", got.Settings)
	}
}
