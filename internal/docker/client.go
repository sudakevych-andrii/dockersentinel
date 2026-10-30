package docker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/moby/moby/client"
)

var (
	ErrPermissionDenied  = errors.New("permission denied to access Docker socket")
	ErrDaemonUnavailable = errors.New("docker daemon is unavailable")
)

// engineAPI is the subset of the Docker SDK used by this package.
// It is satisfied by *client.Client and can be replaced by a fake in tests.
type engineAPI interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	Close() error
}

// EngineInfo describes the connected Docker Engine.
type EngineInfo struct {
	Host       string
	APIVersion string
	OSType     string
}

// Client wraps the Docker SDK client.
type Client struct {
	api     engineAPI
	host    string
	backoff backoff
}

// NewClient creates a client for the given host.
// An empty host means DOCKER_HOST or the default unix socket.
func NewClient(host string) (*Client, error) {
	opts := []client.Opt{client.FromEnv}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}

	api, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &Client{api: api, host: api.DaemonHost(), backoff: defaultBackoff}, nil
}

// Ping checks that the Docker Engine is reachable and negotiates the API version.
func (c *Client) Ping(ctx context.Context) (EngineInfo, error) {
	res, err := c.api.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true})
	if err != nil {
		return EngineInfo{}, c.wrapErr(err)
	}

	return EngineInfo{
		Host:       c.host,
		APIVersion: res.APIVersion,
		OSType:     res.OSType,
	}, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	return c.api.Close()
}

// wrapErr converts low-level SDK errors into errors that are clear to the user.
func (c *Client) wrapErr(err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w (%s): add your user to the 'docker' group and re-login", ErrPermissionDenied, c.host)
	case client.IsErrConnectionFailed(err), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w (%s): %v", ErrDaemonUnavailable, c.host, err)
	default:
		return err
	}
}
