package monitor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
	"github.com/sudakevych-andrii/dockersentinel/internal/projects"
)

// requestTimeout limits a single Docker API request made by Discovery.
const requestTimeout = 10 * time.Second

// ContainerSource is the part of the Docker client used by Discovery.
// It is satisfied by *docker.Client and can be replaced by a fake in tests.
type ContainerSource interface {
	ListContainers(ctx context.Context) ([]docker.Container, error)
	GetContainer(ctx context.Context, id string) (docker.Container, error)
	WatchEvents(ctx context.Context) <-chan docker.Event
}

// Discovery keeps an Inventory in sync with the Docker Engine:
// a full resync after every (re)connect and incremental updates from events.
type Discovery struct {
	src    ContainerSource
	inv    *docker.Inventory
	log    *slog.Logger
	synced bool // the first resync is done
}

// NewDiscovery creates a Discovery that writes into inv.
func NewDiscovery(src ContainerSource, inv *docker.Inventory, log *slog.Logger) *Discovery {
	return &Discovery{src: src, inv: inv, log: log}
}

// Run processes Docker events until ctx is cancelled.
func (d *Discovery) Run(ctx context.Context) {
	for ev := range d.src.WatchEvents(ctx) {
		d.handle(ctx, ev)
	}
}

func (d *Discovery) handle(ctx context.Context, ev docker.Event) {
	switch ev.Kind {
	case docker.EventConnected:
		d.log.Info("docker connected")
		d.resync(ctx)
	case docker.EventDisconnected:
		d.log.Warn("docker unavailable, reconnecting", "err", ev.Err)
	case docker.EventContainer:
		d.apply(ctx, ev)
	}
}

// resync replaces the inventory with the current list of containers.
func (d *Discovery) resync(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	containers, err := d.src.ListContainers(ctx)
	if err != nil {
		d.log.Error("resync failed", "err", err)
		return
	}

	added, removed := d.inv.Replace(containers)

	// On the first sync every container is "added"; a summary is enough.
	if !d.synced {
		d.synced = true
		d.log.Info("inventory synced", "containers", len(containers))
		return
	}

	for _, c := range added {
		d.log.Info("container added", attrs(c)...)
	}
	for _, c := range removed {
		d.log.Info("container removed", attrs(c)...)
	}
	d.log.Info("inventory resynced", "containers", len(containers), "added", len(added), "removed", len(removed))
}

// apply updates a single container after a Docker event.
func (d *Discovery) apply(ctx context.Context, ev docker.Event) {
	d.log.Debug("docker event", "action", ev.Action, "container", ev.Name, "id", docker.ShortID(ev.ContainerID))

	if ev.Action == docker.ActionDestroy {
		d.remove(ev.ContainerID)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	c, err := d.src.GetContainer(ctx, ev.ContainerID)
	if errors.Is(err, docker.ErrContainerNotFound) {
		// Removed before we could read it; destroy may still follow.
		d.remove(ev.ContainerID)
		return
	}
	if err != nil {
		d.log.Warn("refresh container failed", "id", docker.ShortID(ev.ContainerID), "action", ev.Action, "err", err)
		return
	}

	prev, existed := d.inv.Upsert(c)
	switch {
	case !existed:
		d.log.Info("container added", attrs(c)...)
	case prev.State != c.State || prev.Health != c.Health:
		d.log.Info("container changed", append(attrs(c), "prev_state", prev.State, "prev_health", prev.Health)...)
	}
}

func (d *Discovery) remove(id string) {
	if c, ok := d.inv.Remove(id); ok {
		d.log.Info("container removed", attrs(c)...)
	}
}

// attrs returns log attributes that identify a container.
func attrs(c docker.Container) []any {
	project, ok := projects.ProjectName(c)
	if !ok {
		project = projects.StandaloneName
	}
	return []any{
		"container", c.Name,
		"id", docker.ShortID(c.ID),
		"project", project,
		"state", c.State,
		"health", c.Health,
	}
}
