package config

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, data string) Config {
	t.Helper()
	cfg, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg
}

func TestServiceSettingsLayers(t *testing.T) {
	cfg := mustParse(t, `
projects:
  shop:
    health:
      timeout: 5s
    recovery:
      enabled: true
    services:
      web:
        health:
          type: http
          url: http://localhost:8080/health
      db:
        recovery:
          enabled: false
standalone:
  redis:
    health:
      type: tcp
      address: localhost:6379
`)

	web := cfg.ServiceSettings("shop", "web")
	if web.Health.Type != CheckHTTP || web.Health.URL == "" {
		t.Errorf("web: service layer not applied: %+v", web.Health)
	}
	if web.Health.Timeout != 5*time.Second {
		t.Errorf("web: project timeout not inherited: %s", web.Health.Timeout)
	}
	if web.Health.Interval != 15*time.Second || web.Thresholds.CPUPercent != 80 {
		t.Errorf("web: defaults not inherited: %+v", web)
	}
	if !*web.Recovery.Enabled {
		t.Error("web: project recovery.enabled=true not inherited")
	}

	if db := cfg.ServiceSettings("shop", "db"); *db.Recovery.Enabled {
		t.Error("db: service recovery.enabled=false must override project true")
	}

	// A service not listed in the config gets defaults → project.
	if other := cfg.ServiceSettings("shop", "worker"); other.Health.Type != CheckDocker || other.Health.Timeout != 5*time.Second {
		t.Errorf("unlisted service: %+v", other.Health)
	}

	if redis := cfg.ServiceSettings("", "redis"); redis.Health.Type != CheckTCP || redis.Health.Address != "localhost:6379" {
		t.Errorf("standalone redis: %+v", redis.Health)
	}

	if *cfg.ServiceSettings("", "unknown").Recovery.Enabled {
		t.Error("recovery must stay disabled by default")
	}
}

func TestMonitored(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		project   string
		container string
		want      bool
	}{
		{"all mode", ``, "shop", "shop-web-1", true},
		{"all mode standalone", ``, "", "redis", true},
		{"excluded container", "monitoring: {exclude: {containers: [dockersentinel]}}", "", "dockersentinel", false},
		{"excluded project", "monitoring: {exclude: {projects: [shop]}}", "shop", "shop-web-1", false},
		{"disabled project", "projects: {shop: {enabled: false}}", "shop", "shop-web-1", false},
		{"projects mode, listed", "monitoring: {mode: projects}\nprojects: {shop: {}}", "shop", "shop-web-1", true},
		{"projects mode, not listed", "monitoring: {mode: projects}\nprojects: {shop: {}}", "blog", "blog-app-1", false},
		{"projects mode, standalone", "monitoring: {mode: projects}\nprojects: {shop: {}}", "", "redis", false},
		{"containers mode, listed", "monitoring: {mode: containers, containers: [redis]}", "", "redis", true},
		{"containers mode, not listed", "monitoring: {mode: containers, containers: [redis]}", "shop", "shop-web-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mustParse(t, tt.yaml)
			if got := cfg.Monitored(tt.project, tt.container); got != tt.want {
				t.Errorf("Monitored(%q, %q) = %v, want %v", tt.project, tt.container, got, tt.want)
			}
		})
	}
}
