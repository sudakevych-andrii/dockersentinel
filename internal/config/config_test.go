package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEmptyUsesDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.Monitoring.Mode != ModeAll || cfg.Monitoring.MaxConcurrentChecks != 10 {
		t.Errorf("monitoring = %+v", cfg.Monitoring)
	}
	if cfg.Defaults.Health.Type != CheckDocker || cfg.Defaults.Health.Interval != 15*time.Second {
		t.Errorf("defaults.health = %+v", cfg.Defaults.Health)
	}
	if e := cfg.Defaults.Recovery.Enabled; e == nil || *e {
		t.Error("recovery must be disabled by default")
	}
}

func TestParseKeepsDefaultsForMissingKeys(t *testing.T) {
	cfg, err := Parse([]byte(`
defaults:
  health:
    type: http
    timeout: 5s
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	h := cfg.Defaults.Health
	if h.Type != CheckHTTP || h.Timeout != 5*time.Second {
		t.Errorf("overridden fields: %+v", h)
	}
	if h.Interval != 15*time.Second || h.Retries != 3 {
		t.Errorf("missing fields must keep defaults: %+v", h)
	}
	if cfg.Defaults.Thresholds.CPUPercent != 80 {
		t.Errorf("untouched section changed: %+v", cfg.Defaults.Thresholds)
	}
}

func TestParseFullExample(t *testing.T) {
	cfg, err := Parse([]byte(`
docker:
  host: unix:///var/run/docker.sock
monitoring:
  mode: projects
  interval: 10s
  exclude:
    containers: [dockersentinel]
projects:
  ecommerce:
    enabled: true
    thresholds:
      cpu_percent: 95
    services:
      backend:
        health:
          type: http
          url: http://localhost:8080/health
        recovery:
          enabled: true
          max_restarts: 5
      postgres:
        health:
          type: tcp
          address: localhost:5432
standalone:
  redis:
    health:
      type: tcp
      address: localhost:6379
logging:
  level: debug
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.Docker.Host != "unix:///var/run/docker.sock" || cfg.Monitoring.Mode != ModeProjects {
		t.Errorf("top level: docker=%+v mode=%s", cfg.Docker, cfg.Monitoring.Mode)
	}
	if got := cfg.Monitoring.Exclude.Containers; len(got) != 1 || got[0] != "dockersentinel" {
		t.Errorf("exclude.containers = %v", got)
	}

	p := cfg.Projects["ecommerce"]
	if p.Enabled == nil || !*p.Enabled || p.Thresholds.CPUPercent != 95 {
		t.Errorf("project ecommerce = %+v", p)
	}

	backend := p.Services["backend"]
	if backend.Health.Type != CheckHTTP || backend.Health.URL != "http://localhost:8080/health" {
		t.Errorf("backend.health = %+v", backend.Health)
	}
	if e := backend.Recovery.Enabled; e == nil || !*e || backend.Recovery.MaxRestarts != 5 {
		t.Errorf("backend.recovery = %+v", backend.Recovery)
	}
	if p.Services["postgres"].Health.Address != "localhost:5432" {
		t.Errorf("postgres.health = %+v", p.Services["postgres"].Health)
	}
	if cfg.Standalone["redis"].Health.Type != CheckTCP {
		t.Errorf("standalone redis = %+v", cfg.Standalone["redis"])
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("logging.level = %q", cfg.Logging.Level)
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]struct {
		yaml string
		want string
	}{
		"unknown key":      {"monitoring:\n  intervall: 5s\n", "field intervall not found"},
		"invalid duration": {"monitoring:\n  interval: soon\n", "line 2"},
		"wrong type":       {"monitoring:\n  max_concurrent_checks: many\n", "line 2"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("logging:\n  level: warn\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("logging.level = %q, want warn", cfg.Logging.Level)
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("expected error for a missing file")
	}
}

// TestExampleConfig keeps configs/example.yaml loadable and valid.
func TestExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "example.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate:\n%v", err)
	}
}

func TestLocate(t *testing.T) {
	if got := Locate("custom.yaml"); got != "custom.yaml" {
		t.Errorf("explicit path = %q, want custom.yaml", got)
	}

	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := Locate(""); got != "" {
		t.Errorf("no files: got %q, want empty", got)
	}

	if err := os.WriteFile(FileName, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Locate(""); got != FileName {
		t.Errorf("got %q, want %q", got, FileName)
	}
}
