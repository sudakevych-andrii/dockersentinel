package docker

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// stream returns an event stream that delivers msgs and then breaks with io.EOF.
func stream(msgs ...events.Message) client.EventsResult {
	mc := make(chan events.Message)
	ec := make(chan error, 1)
	go func() {
		for _, m := range msgs {
			mc <- m
		}
		ec <- io.EOF
	}()
	return client.EventsResult{Messages: mc, Err: ec}
}

func containerMsg(id string, action events.Action, attrs map[string]string) events.Message {
	return events.Message{
		Type:   events.ContainerEventType,
		Action: action,
		Actor:  events.Actor{ID: id, Attributes: attrs},
		Time:   1761730000,
	}
}

func TestNewEvent(t *testing.T) {
	ev, ok := newEvent(containerMsg("c1", events.ActionDie, map[string]string{"name": "web", "exitCode": "137"}))
	if !ok || ev.Kind != EventContainer || ev.Action != ActionDie || ev.ExitCode != 137 || ev.Name != "web" {
		t.Errorf("die: ok=%v ev=%+v", ok, ev)
	}

	ev, ok = newEvent(containerMsg("c1", events.ActionHealthStatusUnhealthy, nil))
	if !ok || ev.Action != ActionHealthStatus || ev.Health != HealthUnhealthy {
		t.Errorf("health: ok=%v ev=%+v", ok, ev)
	}
	if ev.Attributes == nil {
		t.Error("attributes must not be nil")
	}

	if _, ok := newEvent(containerMsg("c1", "exec_start: sh", nil)); ok {
		t.Error("exec_start must be ignored")
	}
	if _, ok := newEvent(events.Message{Type: events.NetworkEventType, Action: events.ActionCreate}); ok {
		t.Error("network events must be ignored")
	}
}

func TestWatchEventsReconnect(t *testing.T) {
	api := &fakeAPI{
		pingErrs: []error{errors.New("connection refused")},
		streams: []client.EventsResult{
			stream(containerMsg("c1", events.ActionStart, map[string]string{"name": "web"})),
		},
	}
	c := &Client{api: api, backoff: backoff{initial: time.Millisecond, limit: 2 * time.Millisecond}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.WatchEvents(ctx)

	want := []struct {
		kind   EventKind
		action Action
	}{
		{EventDisconnected, ""}, // first ping fails
		{EventConnected, ""},    // ping succeeds, subscribed
		{EventContainer, ActionStart},
		{EventDisconnected, ""}, // stream breaks with EOF
		{EventConnected, ""},    // reconnected
	}
	for i, w := range want {
		ev := next(t, ch)
		if ev.Kind != w.kind || ev.Action != w.action {
			t.Fatalf("event[%d] = %s/%s, want %s/%s", i, ev.Kind, ev.Action, w.kind, w.action)
		}
		if ev.Kind == EventDisconnected && ev.Err == nil {
			t.Errorf("event[%d]: disconnected without error", i)
		}
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("unexpected event after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed after cancel")
	}
}

func next(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		return ev
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
	return Event{}
}
