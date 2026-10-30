package docker

import (
	"context"
	"errors"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// fakeAPI is an in-memory engineAPI for unit tests.
type fakeAPI struct {
	summaries []container.Summary
	inspects  map[string]container.InspectResponse
	listErr   error
	pingErrs  []error               // returned by successive Ping calls, then nil
	streams   []client.EventsResult // returned by successive Events calls
}

func (f *fakeAPI) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	if len(f.pingErrs) > 0 {
		err := f.pingErrs[0]
		f.pingErrs = f.pingErrs[1:]
		return client.PingResult{}, err
	}
	return client.PingResult{}, nil
}

// ContainerList supports only the "id" filter, by exact match.
func (f *fakeAPI) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	ids := opts.Filters["id"]
	if len(ids) == 0 {
		return client.ContainerListResult{Items: f.summaries}, f.listErr
	}

	var items []container.Summary
	for _, s := range f.summaries {
		if ids[s.ID] {
			items = append(items, s)
		}
	}
	return client.ContainerListResult{Items: items}, f.listErr
}

func (f *fakeAPI) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	inspect, ok := f.inspects[id]
	if !ok {
		return client.ContainerInspectResult{}, cerrdefs.ErrNotFound
	}
	return client.ContainerInspectResult{Container: inspect}, nil
}

// Events returns the prepared streams in order, then a stream that never ends.
func (f *fakeAPI) Events(context.Context, client.EventsListOptions) client.EventsResult {
	if len(f.streams) > 0 {
		s := f.streams[0]
		f.streams = f.streams[1:]
		return s
	}
	return client.EventsResult{Messages: make(chan events.Message), Err: make(chan error)}
}

func (f *fakeAPI) Close() error { return nil }

func TestListContainers(t *testing.T) {
	api := &fakeAPI{
		summaries: []container.Summary{
			{
				ID:     "bbb",
				Names:  []string{"/web"},
				State:  container.StateRunning,
				Health: &container.HealthSummary{Status: container.Healthy},
				Labels: map[string]string{"com.docker.compose.project": "shop"},
			},
			{ID: "aaa", Names: []string{"/api"}, State: container.StateExited},
			{ID: "gone", Names: []string{"/removed"}, State: container.StateRunning},
		},
		inspects: map[string]container.InspectResponse{
			"aaa": {RestartCount: 2, State: &container.State{ExitCode: 1, StartedAt: "0001-01-01T00:00:00Z"}},
			"bbb": {State: &container.State{StartedAt: "2026-10-29T10:00:00.5Z"}},
		},
	}
	c := &Client{api: api}

	got, err := c.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d containers, want 2 (removed container must be skipped)", len(got))
	}
	if got[0].Name != "api" || got[1].Name != "web" {
		t.Errorf("names = %q, %q; want sorted api, web", got[0].Name, got[1].Name)
	}

	api0 := got[0]
	if api0.State != StateExited || api0.ExitCode != 1 || api0.RestartCount != 2 {
		t.Errorf("api = %+v", api0)
	}
	if api0.Health != HealthNone {
		t.Errorf("api health = %q, want %q", api0.Health, HealthNone)
	}
	if !api0.StartedAt.IsZero() {
		t.Errorf("api StartedAt = %v, want zero", api0.StartedAt)
	}
	if api0.Labels == nil {
		t.Error("api labels must not be nil")
	}

	web := got[1]
	if web.Health != HealthHealthy {
		t.Errorf("web health = %q, want %q", web.Health, HealthHealthy)
	}
	if web.StartedAt.IsZero() {
		t.Error("web StartedAt must be parsed")
	}
}

func TestListContainersListError(t *testing.T) {
	c := &Client{api: &fakeAPI{listErr: errors.New("boom")}}

	if _, err := c.ListContainers(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetContainer(t *testing.T) {
	api := &fakeAPI{
		summaries: []container.Summary{
			{ID: "aaa", Names: []string{"/api"}, State: container.StateRunning},
			{ID: "bbb", Names: []string{"/web"}, State: container.StateRunning},
		},
		inspects: map[string]container.InspectResponse{
			"aaa": {State: &container.State{}},
			"bbb": {State: &container.State{}},
		},
	}
	c := &Client{api: api}

	got, err := c.GetContainer(context.Background(), "bbb")
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if got.Name != "web" {
		t.Errorf("name = %q, want web", got.Name)
	}

	if _, err := c.GetContainer(context.Background(), "gone"); !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("err = %v, want ErrContainerNotFound", err)
	}
}
