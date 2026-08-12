package config

import "testing"

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("TARGET_DRONE_COUNT", "10000")
	t.Setenv("UPDATE_INTERVAL_MS", "300")
	t.Setenv("BATCH_SIZE", "5000")
	t.Setenv("RETENTION_DAYS", "7")
	t.Setenv("MEMORY_HISTORY_POINTS", "500")
	t.Setenv("SSE_INTERVAL_MS", "300")
	t.Setenv("NATS_MAX_BYTES", "134217728")
	t.Setenv("HISTORY_SAMPLE_INTERVAL", "5m")
}

func TestLoadAcceptsResearchLimits(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TargetDroneCount != 10_000 || cfg.UpdateIntervalMS != 300 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
}

func TestLoadRejectsInvalidDroneCountAndInterval(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "too many drones", key: "TARGET_DRONE_COUNT", value: "10001"},
		{name: "tick too fast", key: "UPDATE_INTERVAL_MS", value: "99"},
		{name: "invalid sample duration", key: "HISTORY_SAMPLE_INTERVAL", value: "five minutes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted %s=%q", test.key, test.value)
			}
		})
	}
}
