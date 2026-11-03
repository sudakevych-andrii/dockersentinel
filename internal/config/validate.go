package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// Validate checks the configuration and returns all problems at once,
// each prefixed with the YAML path, e.g. "projects.shop.services.web.health.url: ...".
func (c Config) Validate() error {
	v := &validator{}

	c.validateMonitoring(v)
	c.validateSettings(v)

	if c.Storage.Path == "" {
		v.add("storage.path", "must not be empty")
	}
	if c.Storage.Retention <= 0 {
		v.add("storage.retention", "must be positive")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(c.Logging.Level)); err != nil {
		v.add("logging.level", "must be one of debug, info, warn, error")
	}

	return v.err()
}

// LogLevel returns logging.level as an slog level. It assumes a validated config.
func (c Config) LogLevel() slog.Level {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Logging.Level))
	return level
}

func (c Config) validateMonitoring(v *validator) {
	m := c.Monitoring

	switch m.Mode {
	case ModeAll:
	case ModeProjects:
		if !slices.ContainsFunc(sortedKeys(c.Projects), func(name string) bool {
			e := c.Projects[name].Enabled
			return e == nil || *e
		}) {
			v.add("monitoring.mode", "projects mode requires at least one enabled project in the projects section")
		}
	case ModeContainers:
		if len(m.Containers) == 0 {
			v.add("monitoring.containers", "must not be empty in containers mode")
		}
	default:
		v.add("monitoring.mode", fmt.Sprintf("unknown mode %q, want all, projects or containers", m.Mode))
	}

	if m.Interval <= 0 {
		v.add("monitoring.interval", "must be positive")
	}
	if m.MaxConcurrentChecks < 1 {
		v.add("monitoring.max_concurrent_checks", "must be at least 1")
	}
}

// validateSettings checks every settings layer. Range checks apply to the
// values a layer sets itself; required fields are checked on the effective
// settings where the layer sets the check type, so an error is reported once.
func (c Config) validateSettings(v *validator) {
	c.validateLayer(v, "defaults", c.Defaults, c.Defaults)

	for _, name := range sortedKeys(c.Projects) {
		p := c.Projects[name]
		path := "projects." + name
		c.validateLayer(v, path, p.Settings, c.Defaults.merge(p.Settings))

		for _, svc := range sortedKeys(p.Services) {
			c.validateLayer(v, path+".services."+svc, p.Services[svc], c.ServiceSettings(name, svc))
		}
	}

	for _, name := range sortedKeys(c.Standalone) {
		c.validateLayer(v, "standalone."+name, c.Standalone[name], c.ServiceSettings("", name))
	}
}

func (c Config) validateLayer(v *validator, path string, own, effective Settings) {
	h := own.Health
	hp := path + ".health"

	switch h.Type {
	case "", CheckDocker:
	case CheckHTTP:
		if effective.Health.URL == "" {
			v.add(hp+".url", "required for type http")
		}
	case CheckTCP:
		if effective.Health.Address == "" {
			v.add(hp+".address", "required for type tcp")
		}
	default:
		v.add(hp+".type", fmt.Sprintf("unknown type %q, want docker, http or tcp", h.Type))
	}

	if h.URL != "" {
		if u, err := url.Parse(h.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			v.add(hp+".url", fmt.Sprintf("invalid URL %q, want http(s)://host[:port]/path", h.URL))
		}
	}
	if h.Address != "" {
		if _, port, err := net.SplitHostPort(h.Address); err != nil || !validPort(port) {
			v.add(hp+".address", fmt.Sprintf("invalid address %q, want host:port", h.Address))
		}
	}
	if h.ExpectedStatus != 0 && (h.ExpectedStatus < 100 || h.ExpectedStatus > 599) {
		v.add(hp+".expected_status", "must be an HTTP status code (100-599)")
	}
	nonNegative(v, hp+".interval", h.Interval)
	nonNegative(v, hp+".timeout", h.Timeout)
	if h.Retries < 0 {
		v.add(hp+".retries", "must not be negative")
	}
	if eh := effective.Health; (h.Timeout != 0 || h.Interval != 0) && eh.Timeout >= eh.Interval {
		v.add(hp+".timeout", fmt.Sprintf("must be less than interval (%s >= %s)", eh.Timeout, eh.Interval))
	}

	t := own.Thresholds
	tp := path + ".thresholds"
	if t.CPUPercent < 0 {
		v.add(tp+".cpu_percent", "must not be negative")
	}
	if t.MemoryPercent < 0 || t.MemoryPercent > 100 {
		v.add(tp+".memory_percent", "must be between 0 and 100")
	}
	if t.Consecutive < 0 {
		v.add(tp+".consecutive", "must not be negative")
	}

	r := own.Recovery
	rp := path + ".recovery"
	if r.MaxRestarts < 0 {
		v.add(rp+".max_restarts", "must not be negative")
	}
	nonNegative(v, rp+".window", r.Window)
	nonNegative(v, rp+".cooldown", r.Cooldown)
}

func nonNegative(v *validator, path string, d time.Duration) {
	if d < 0 {
		v.add(path, "must not be negative")
	}
}

func validPort(s string) bool {
	p, err := strconv.Atoi(s)
	return err == nil && p >= 1 && p <= 65535
}

// validator collects validation problems.
type validator struct {
	errs []error
}

func (v *validator) add(path, msg string) {
	v.errs = append(v.errs, fmt.Errorf("%s: %s", path, msg))
}

// err joins all problems; each one is on its own line.
func (v *validator) err() error {
	return errors.Join(v.errs...)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
