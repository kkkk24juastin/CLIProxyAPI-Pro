package embeddedusage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBPath       string
	LegacyDBPath string
	BatchSize    int
	PollInterval time.Duration
	QueryLimit   int
}

const usageEventsPageLimit = 5000
const usageEventsSentinelLimit = usageEventsPageLimit + 1

func LoadConfig() Config {
	dataDir := env("USAGE_DATA_DIR", "/CLIProxyAPI/usage")
	legacyDBPath := env("USAGE_DB_PATH", filepath.Join(dataDir, "usage.sqlite"))
	return Config{
		DBPath:       env("PRO_OBSERVABILITY_DB_PATH", legacyDBPath),
		LegacyDBPath: legacyDBPath,
		BatchSize:    envInt("USAGE_BATCH_SIZE", 100),
		PollInterval: time.Duration(envInt("USAGE_POLL_INTERVAL_MS", 500)) * time.Millisecond,
		QueryLimit:   envInt("USAGE_QUERY_LIMIT", 50000),
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
