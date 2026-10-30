package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
)

// fakeSource is an in-memory ContainerSource for unit tests.
type fakeSource struct {
	containers map[string]docker.Container
	events     chan docker.Event
}

func (f *fakeSource) ListContainers(context.Context) ([]docker.Container, error) {
	cs := make([]docker.Container, 0, len(f.containers))
	for _, c := range f.containers {
		cs = append(cs, c)
	}
	return cs, nil
}

func (f *fakeSource) GetContainer(_ context.Context, id string) (docker.Container, error) {
	c, ok := f.containers[id]
	if !ok {
		return docker.Container{}, fmt.Errorf("%w: %s", docker.ErrContainerNotFound, id)
	}
	return c, nil
}

func (f *fakeSource) WatchEvents(context.Context) <-chan docker.Event {
	return f.events
}

func ctr(id, name string, state docker.State) docker.Container {
	return docker.Container{ID: id, Name: name, State: state, Labels: map[string]string{}}
}

func containerEvent(id string, action docker.Action) docker.Event {
	return docker.Event{Kind: docker.EventContainer, ContainerID: id, Action: action}
}

func newTestDiscovery(src *fakeSource) *Discovery {
	return NewDiscovery(src, docker.NewInventory(), slog.New(slog.DiscardHandler))
}

func inventoryNames(d *Discovery) string {
	var names []string
	for _, c := range d.inv.List() {
		names = append(names, c.Name)
	}
	return fmt.Sprint(names)
}

func TestDiscoveryHandle(t *testing.T) {
	src := &fakeSource{containers: map[string]docker.Container{
		"a": ctr("a", "api", docker.StateRunning),
		"b": ctr("b", "web", docker.StateRunning),
	}}
	d := newTestDiscovery(src)
	ctx := context.Background()

	d.handle(ctx, docker.Event{Kind: docker.EventConnected})
	if got := inventoryNames(d); got != "[api web]" {
		t.Fatalf("after initial sync: %s, want [api web]", got)
	}

	// A new container appears.
	src.containers["c"] = ctr("c", "db", docker.StateRunning)
	d.handle(ctx, containerEvent("c", docker.ActionStart))

	// A container stops.
	src.containers["b"] = ctr("b", "web", docker.StateExited)
	d.handle(ctx, containerEvent("b", docker.ActionDie))

	// A container is removed.
	delete(src.containers, "a")
	d.handle(ctx, containerEvent("a", docker.ActionDestroy))

	if got := inventoryNames(d); got != "[db web]" {
		t.Errorf("after events: %s, want [db web]", got)
	}
	if c, _ := d.inv.Get("b"); c.State != docker.StateExited {
		t.Errorf("web state = %s, want exited", c.State)
	}

	// A container is removed while Docker is unreachable; the resync finds it.
	delete(src.containers, "c")
	d.handle(ctx, docker.Event{Kind: docker.EventDisconnected})
	d.handle(ctx, docker.Event{Kind: docker.EventConnected})
	if got := inventoryNames(d); got != "[web]" {
		t.Errorf("after resync: %s, want [web]", got)
	}

	// An event for a container that is already gone changes nothing.
	d.handle(ctx, containerEvent("gone", docker.ActionStart))
	if got := inventoryNames(d); got != "[web]" {
		t.Errorf("after event for removed container: %s, want [web]", got)
	}
}

func TestDiscoveryRunStopsWhenEventsClose(t *testing.T) {
	src := &fakeSource{
		containers: map[string]docker.Container{"a": ctr("a", "api", docker.StateRunning)},
		events:     make(chan docker.Event),
	}
	d := newTestDiscovery(src)

	done := make(chan struct{})
	go func() {
		d.Run(context.Background())
		close(done)
	}()

	src.events <- docker.Event{Kind: docker.EventConnected}
	close(src.events)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the event channel was closed")
	}
	if got := inventoryNames(d); got != "[api]" {
		t.Errorf("inventory = %s, want [api]", got)
	}
}
