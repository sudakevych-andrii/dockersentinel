package config

import "slices"

// ServiceSettings returns the effective settings of a container.
// Compose containers get defaults → project → service; standalone containers
// (empty project) get defaults → standalone, where service is the container name.
func (c Config) ServiceSettings(project, service string) Settings {
	s := c.Defaults

	if project == "" {
		return s.merge(c.Standalone[service])
	}

	p := c.Projects[project]
	return s.merge(p.Settings).merge(p.Services[service])
}

// Monitored reports whether a container is monitored according to
// monitoring.mode, monitoring.exclude and project "enabled" flags.
// project is empty for standalone containers; container is the container name.
func (c Config) Monitored(project, container string) bool {
	m := c.Monitoring

	if slices.Contains(m.Exclude.Containers, container) {
		return false
	}
	if project != "" {
		if slices.Contains(m.Exclude.Projects, project) {
			return false
		}
		if p, ok := c.Projects[project]; ok && p.Enabled != nil && !*p.Enabled {
			return false
		}
	}

	switch m.Mode {
	case ModeProjects:
		_, ok := c.Projects[project]
		return project != "" && ok
	case ModeContainers:
		return slices.Contains(m.Containers, container)
	default:
		return true
	}
}

// merge returns s overridden by the non-zero fields of o.
func (s Settings) merge(o Settings) Settings {
	h := &s.Health
	set(&h.Type, o.Health.Type)
	set(&h.Interval, o.Health.Interval)
	set(&h.Timeout, o.Health.Timeout)
	set(&h.Retries, o.Health.Retries)
	set(&h.URL, o.Health.URL)
	set(&h.ExpectedStatus, o.Health.ExpectedStatus)
	set(&h.Address, o.Health.Address)

	t := &s.Thresholds
	set(&t.CPUPercent, o.Thresholds.CPUPercent)
	set(&t.MemoryPercent, o.Thresholds.MemoryPercent)
	set(&t.Consecutive, o.Thresholds.Consecutive)

	r := &s.Recovery
	if o.Recovery.Enabled != nil {
		r.Enabled = o.Recovery.Enabled
	}
	set(&r.MaxRestarts, o.Recovery.MaxRestarts)
	set(&r.Window, o.Recovery.Window)
	set(&r.Cooldown, o.Recovery.Cooldown)

	return s
}

// set overwrites *dst with v unless v is the zero value ("inherit").
func set[T comparable](dst *T, v T) {
	var zero T
	if v != zero {
		*dst = v
	}
}
