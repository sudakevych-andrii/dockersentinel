package monitor

import (
	"github.com/sudakevych-andrii/dockersentinel/internal/config"
	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
	"github.com/sudakevych-andrii/dockersentinel/internal/projects"
)

// Filter reports whether a container is monitored. A nil Filter matches everything.
type Filter func(docker.Container) bool

// NewFilter builds a Filter from monitoring.mode, monitoring.exclude and project "enabled" flags.
func NewFilter(cfg config.Config) Filter {
	return func(c docker.Container) bool {
		project, _ := projects.ProjectName(c)
		return cfg.Monitored(project, c.Name)
	}
}

// Match reports whether c is monitored.
func (f Filter) Match(c docker.Container) bool {
	return f == nil || f(c)
}

// Apply returns the monitored containers, keeping their order.
func (f Filter) Apply(cs []docker.Container) []docker.Container {
	if f == nil {
		return cs
	}

	out := make([]docker.Container, 0, len(cs))
	for _, c := range cs {
		if f(c) {
			out = append(out, c)
		}
	}
	return out
}
