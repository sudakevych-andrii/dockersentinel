package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// Mode selects which containers are monitored.
type Mode string

const (
	ModeAll        Mode = "all"        // every container of the Engine
	ModeProjects   Mode = "projects"   // only enabled projects from the projects section
	ModeContainers Mode = "containers" // only containers listed in monitoring.containers
)

// CheckType is the kind of health check.
type CheckType string

const (
	CheckDocker CheckType = "docker"
	CheckHTTP   CheckType = "http"
	CheckTCP    CheckType = "tcp"
)

// Config is the full DockerSentinel configuration.
type Config struct {
	Docker     Docker              `yaml:"docker"`
	Monitoring Monitoring          `yaml:"monitoring"`
	Defaults   Settings            `yaml:"defaults"`
	Projects   map[string]Project  `yaml:"projects"`
	Standalone map[string]Settings `yaml:"standalone"` // keyed by container name
	Storage    Storage             `yaml:"storage"`
	Logging    Logging             `yaml:"logging"`
}

// Docker configures the connection to the Docker Engine.
type Docker struct {
	Host string `yaml:"host"` // empty means DOCKER_HOST or the default unix socket
}

// Monitoring configures what is monitored and how often.
type Monitoring struct {
	Mode                Mode          `yaml:"mode"`
	Interval            time.Duration `yaml:"interval"`
	MaxConcurrentChecks int           `yaml:"max_concurrent_checks"`
	Containers          []string      `yaml:"containers"` // used by ModeContainers
	Exclude             Exclude       `yaml:"exclude"`
}

// Exclude lists projects and containers that are never monitored.
type Exclude struct {
	Projects   []string `yaml:"projects"`
	Containers []string `yaml:"containers"`
}

// Settings are per-service monitoring settings, layered as defaults → project → service.
// A zero value means "inherit from the outer layer".
type Settings struct {
	Health     Health     `yaml:"health"`
	Thresholds Thresholds `yaml:"thresholds"`
	Recovery   Recovery   `yaml:"recovery"`
}

// Health configures the health check of a service.
type Health struct {
	Type           CheckType     `yaml:"type"`
	Interval       time.Duration `yaml:"interval"`
	Timeout        time.Duration `yaml:"timeout"`
	Retries        int           `yaml:"retries"`
	URL            string        `yaml:"url"`             // http
	ExpectedStatus int           `yaml:"expected_status"` // http
	Address        string        `yaml:"address"`         // tcp, host:port reachable from the host
}

// Thresholds configures resource incidents.
type Thresholds struct {
	CPUPercent    float64 `yaml:"cpu_percent"`
	MemoryPercent float64 `yaml:"memory_percent"`
	Consecutive   int     `yaml:"consecutive"` // exceedances in a row before an incident
}

// Recovery configures automatic restarts. It is disabled by default.
type Recovery struct {
	Enabled     *bool         `yaml:"enabled"` // pointer: false must override an inherited true
	MaxRestarts int           `yaml:"max_restarts"`
	Window      time.Duration `yaml:"window"`
	Cooldown    time.Duration `yaml:"cooldown"`
}

// Project configures a Compose project and its services.
type Project struct {
	Enabled  *bool `yaml:"enabled"` // nil means true
	Settings `yaml:",inline"`
	Services map[string]Settings `yaml:"services"`
}

// Storage configures the SQLite database.
type Storage struct {
	Path      string        `yaml:"path"`
	Retention time.Duration `yaml:"retention"` // how long detailed check results are kept
}

// Logging configures log output.
type Logging struct {
	Level string `yaml:"level"` // debug | info | warn | error
}

// Default returns the configuration used when no config file is given.
func Default() Config {
	disabled := false

	return Config{
		Monitoring: Monitoring{
			Mode:                ModeAll,
			Interval:            5 * time.Second,
			MaxConcurrentChecks: 10,
		},
		Defaults: Settings{
			Health: Health{
				Type:           CheckDocker,
				Interval:       15 * time.Second,
				Timeout:        3 * time.Second,
				Retries:        3,
				ExpectedStatus: 200,
			},
			Thresholds: Thresholds{
				CPUPercent:    80,
				MemoryPercent: 90,
				Consecutive:   3,
			},
			Recovery: Recovery{
				Enabled:     &disabled,
				MaxRestarts: 3,
				Window:      10 * time.Minute,
				Cooldown:    60 * time.Second,
			},
		},
		Storage: Storage{
			Path:      "./dockersentinel.db",
			Retention: 7 * 24 * time.Hour,
		},
		Logging: Logging{Level: "info"},
	}
}

// FileName is the config file looked up in the working directory.
const FileName = "dockersentinel.yaml"

// Locate returns the config file to use: explicit if given, otherwise the first
// existing of ./dockersentinel.yaml and $XDG_CONFIG_HOME/dockersentinel/config.yaml.
// An empty result means no file was found and the defaults apply.
func Locate(explicit string) string {
	if explicit != "" {
		return explicit
	}

	candidates := []string{FileName}
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "dockersentinel", "config.yaml"))
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Load reads the config file at path on top of the defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes YAML on top of the defaults. Keys missing from data keep their
// default values; unknown keys are an error, so typos do not go unnoticed.
func Parse(data []byte) (Config, error) {
	cfg := Default()

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, err
	}
	return cfg, nil
}
