package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// pingTimeout limits the reachability check before each subscription.
const pingTimeout = 5 * time.Second

// EventKind tells what an Event describes.
type EventKind string

const (
	// EventConnected is sent after the event stream is (re)established.
	// Events may have been missed before it, so the consumer must do a full resync.
	EventConnected EventKind = "connected"
	// EventDisconnected is sent once when the Docker Engine becomes unreachable.
	EventDisconnected EventKind = "disconnected"
	// EventContainer is a lifecycle event of a single container.
	EventContainer EventKind = "container"
)

// Action is a container lifecycle action.
type Action string

const (
	ActionCreate       Action = "create"
	ActionStart        Action = "start"
	ActionStop         Action = "stop"
	ActionDie          Action = "die"
	ActionDestroy      Action = "destroy"
	ActionRestart      Action = "restart"
	ActionHealthStatus Action = "health_status"
)

// watchedActions are the container actions DockerSentinel subscribes to.
var watchedActions = []Action{
	ActionCreate, ActionStart, ActionStop, ActionDie,
	ActionDestroy, ActionRestart, ActionHealthStatus,
}

// Event is a message from the Docker event stream.
type Event struct {
	Kind EventKind
	Time time.Time
	Err  error // set for EventDisconnected

	// Fields below are set for EventContainer.
	ContainerID string
	Name        string
	Action      Action
	Health      Health // set for ActionHealthStatus
	ExitCode    int    // set for ActionDie
	// Attributes holds container labels plus Docker attributes like name and image.
	Attributes map[string]string
}

// backoff is a bounded exponential delay between reconnect attempts.
type backoff struct {
	initial time.Duration
	limit   time.Duration
}

var defaultBackoff = backoff{initial: time.Second, limit: 30 * time.Second}

func (b backoff) next(d time.Duration) time.Duration {
	return min(d*2, b.limit)
}

// WatchEvents streams container events until ctx is cancelled, then closes the channel.
// When the stream breaks, it sends EventDisconnected once and reconnects with
// exponential backoff. EventConnected is sent after every successful (re)connect.
func (c *Client) WatchEvents(ctx context.Context) <-chan Event {
	out := make(chan Event)

	go func() {
		defer close(out)

		delay := c.backoff.initial
		down := false

		for {
			connected, err := c.streamEvents(ctx, out)
			if ctx.Err() != nil {
				return
			}

			if connected {
				delay = c.backoff.initial
				down = false
			}
			if !down {
				down = true
				ev := Event{Kind: EventDisconnected, Time: time.Now().UTC(), Err: c.wrapErr(err)}
				if !send(ctx, out, ev) {
					return
				}
			}

			if !sleep(ctx, delay) {
				return
			}
			delay = c.backoff.next(delay)
		}
	}()

	return out
}

// streamEvents subscribes to the event stream and forwards events to out until the stream fails.
// connected reports whether the subscription was established.
func (c *Client) streamEvents(ctx context.Context, out chan<- Event) (connected bool, err error) {
	pingCtx, cancelPing := context.WithTimeout(ctx, pingTimeout)
	_, err = c.api.Ping(pingCtx, client.PingOptions{NegotiateAPIVersion: true})
	cancelPing()
	if err != nil {
		return false, err
	}

	// Cancelling ctx stops the SDK goroutine that reads the stream.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	filters := make(client.Filters).Add("type", string(events.ContainerEventType))
	for _, a := range watchedActions {
		filters.Add("event", string(a))
	}
	res := c.api.Events(ctx, client.EventsListOptions{Filters: filters})

	// Connected is sent after subscribing: the consumer resyncs on it,
	// so no change between the resync and the first event is lost.
	if !send(ctx, out, Event{Kind: EventConnected, Time: time.Now().UTC()}) {
		return true, ctx.Err()
	}

	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case m := <-res.Messages:
			ev, ok := newEvent(m)
			if ok && !send(ctx, out, ev) {
				return true, ctx.Err()
			}
		case err := <-res.Err:
			if err == nil || errors.Is(err, io.EOF) {
				err = fmt.Errorf("%w: event stream closed", ErrDaemonUnavailable)
			}
			return true, err
		}
	}
}

// newEvent converts an SDK message. ok is false for events DockerSentinel does not watch.
func newEvent(m events.Message) (ev Event, ok bool) {
	if m.Type != events.ContainerEventType {
		return Event{}, false
	}

	action, health := parseAction(m.Action)
	if !slices.Contains(watchedActions, action) {
		return Event{}, false
	}

	ev = Event{
		Kind:        EventContainer,
		Time:        eventTime(m),
		ContainerID: m.Actor.ID,
		Name:        m.Actor.Attributes["name"],
		Action:      action,
		Health:      health,
		Attributes:  m.Actor.Attributes,
	}
	if action == ActionDie {
		ev.ExitCode, _ = strconv.Atoi(m.Actor.Attributes["exitCode"])
	}
	if ev.Attributes == nil {
		ev.Attributes = map[string]string{}
	}

	return ev, true
}

// parseAction splits "health_status: healthy" into the action and the health status.
func parseAction(a events.Action) (Action, Health) {
	if rest, ok := strings.CutPrefix(string(a), string(ActionHealthStatus)); ok {
		return ActionHealthStatus, Health(strings.TrimPrefix(rest, ": "))
	}
	return Action(a), ""
}

func eventTime(m events.Message) time.Time {
	if m.TimeNano != 0 {
		return time.Unix(0, m.TimeNano).UTC()
	}
	return time.Unix(m.Time, 0).UTC()
}

// send delivers ev unless ctx is done first.
func send(ctx context.Context, out chan<- Event, ev Event) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// sleep waits for d unless ctx is done first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
