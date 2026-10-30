package docker

import (
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
)

// State is the container state reported by Docker.
type State string

const (
	StateCreated    State = "created"
	StateRunning    State = "running"
	StatePaused     State = "paused"
	StateRestarting State = "restarting"
	StateRemoving   State = "removing"
	StateExited     State = "exited"
	StateDead       State = "dead"
)

// Health is the status of the built-in Docker HEALTHCHECK.
type Health string

const (
	HealthNone      Health = "none" // no HEALTHCHECK in the image
	HealthStarting  Health = "starting"
	HealthHealthy   Health = "healthy"
	HealthUnhealthy Health = "unhealthy"
)

// Port is a published or exposed container port.
type Port struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Protocol    string `json:"protocol"`
}

// Container is the domain model of a Docker container.
// It does not depend on SDK types, so other packages can use it freely.
type Container struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	State        State             `json:"state"`
	Status       string            `json:"status"`
	Health       Health            `json:"health"`
	ExitCode     int               `json:"exit_code"`
	RestartCount int               `json:"restart_count"`
	Ports        []Port            `json:"ports"`
	Labels       map[string]string `json:"labels"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    time.Time         `json:"started_at"`
}

// newContainer builds a Container from the list summary and the inspect details.
func newContainer(s container.Summary, inspect container.InspectResponse) Container {
	c := Container{
		ID:           s.ID,
		Name:         containerName(s.Names),
		Image:        s.Image,
		State:        State(s.State),
		Status:       s.Status,
		Health:       HealthNone,
		RestartCount: inspect.RestartCount,
		Ports:        make([]Port, 0, len(s.Ports)),
		Labels:       s.Labels,
		CreatedAt:    time.Unix(s.Created, 0).UTC(),
	}

	if s.Health != nil && s.Health.Status != "" {
		c.Health = Health(s.Health.Status)
	}

	if st := inspect.State; st != nil {
		c.ExitCode = st.ExitCode
		c.StartedAt = parseDockerTime(st.StartedAt)
	}

	for _, p := range s.Ports {
		port := Port{
			PrivatePort: p.PrivatePort,
			PublicPort:  p.PublicPort,
			Protocol:    p.Type,
		}
		if p.IP.IsValid() {
			port.IP = p.IP.String()
		}
		c.Ports = append(c.Ports, port)
	}

	if c.Labels == nil {
		c.Labels = map[string]string{}
	}

	return c
}

// containerName returns the primary name without the leading slash.
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// parseDockerTime parses Docker timestamps; Docker uses "0001-01-01T00:00:00Z" for "never".
func parseDockerTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil || t.Year() <= 1 {
		return time.Time{}
	}
	return t
}
