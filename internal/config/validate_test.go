package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestValidateDefaults(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config must be valid: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := map[string]struct {
		yaml string
		want string
	}{
		"unknown mode":         {"monitoring: {mode: some}", "monitoring.mode: unknown mode"},
		"containers mode":      {"monitoring: {mode: containers}", "monitoring.containers: must not be empty"},
		"projects mode":        {"monitoring: {mode: projects}", "monitoring.mode: projects mode requires"},
		"zero concurrency":     {"monitoring: {max_concurrent_checks: 0}", "monitoring.max_concurrent_checks"},
		"bad log level":        {"logging: {level: loud}", "logging.level"},
		"unknown check type":   {"defaults: {health: {type: ping}}", "defaults.health.type: unknown type"},
		"http without url":     {"projects: {shop: {services: {web: {health: {type: http}}}}}", "projects.shop.services.web.health.url: required for type http"},
		"tcp without address":  {"standalone: {redis: {health: {type: tcp}}}", "standalone.redis.health.address: required for type tcp"},
		"bad url":              {"defaults: {health: {type: http, url: 'localhost:8080'}}", "defaults.health.url: invalid URL"},
		"bad address":          {"defaults: {health: {type: tcp, address: localhost}}", "defaults.health.address: invalid address"},
		"bad port":             {"defaults: {health: {type: tcp, address: 'localhost:99999'}}", "defaults.health.address: invalid address"},
		"bad status":           {"defaults: {health: {expected_status: 42}}", "defaults.health.expected_status"},
		"timeout >= interval":  {"projects: {shop: {health: {timeout: 30s}}}", "projects.shop.health.timeout: must be less than interval"},
		"memory over 100":      {"defaults: {thresholds: {memory_percent: 120}}", "defaults.thresholds.memory_percent"},
		"negative restarts":    {"defaults: {recovery: {max_restarts: -1}}", "defaults.recovery.max_restarts"},
		"negative cooldown":    {"defaults: {recovery: {cooldown: -1s}}", "defaults.recovery.cooldown"},
		"empty storage path":   {"storage: {path: ''}", "storage.path: must not be empty"},
		"invalid retention":    {"storage: {retention: -1h}", "storage.retention: must be positive"},
		"inherited url is set": {"defaults: {health: {url: 'http://localhost/health'}}\nprojects: {shop: {health: {type: http}}}", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := mustParse(t, tt.yaml).Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestValidateReportsAllErrorsOnce(t *testing.T) {
	err := mustParse(t, `
monitoring:
  interval: -1s
defaults:
  health:
    type: http
`).Validate()
	if err == nil {
		t.Fatal("expected errors")
	}

	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d errors, want 2 (interval, url):\n%v", len(lines), err)
	}
}

func TestLogLevel(t *testing.T) {
	if got := mustParse(t, "logging: {level: debug}").LogLevel(); got != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", got)
	}
}
