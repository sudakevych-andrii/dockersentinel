package docker

import (
	"context"
	"errors"
	"fmt"
	"sort"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// ErrContainerNotFound is returned when a container no longer exists.
var ErrContainerNotFound = errors.New("container not found")

// ListContainers returns all containers, including stopped ones, sorted by name.
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	containers, err := c.listContainers(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	sortByName(containers)
	return containers, nil
}

// GetContainer returns a single container by its full ID.
// It returns ErrContainerNotFound if the container was removed.
func (c *Client) GetContainer(ctx context.Context, id string) (Container, error) {
	containers, err := c.listContainers(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("id", id),
	})
	if err != nil {
		return Container{}, err
	}

	// The id filter matches by prefix, so look for the exact ID.
	for _, ct := range containers {
		if ct.ID == id {
			return ct, nil
		}
	}
	return Container{}, fmt.Errorf("%w: %s", ErrContainerNotFound, ShortID(id))
}

// listContainers lists containers and enriches each one with inspect details.
func (c *Client) listContainers(ctx context.Context, opts client.ContainerListOptions) ([]Container, error) {
	list, err := c.api.ContainerList(ctx, opts)
	if err != nil {
		return nil, c.wrapErr(err)
	}

	containers := make([]Container, 0, len(list.Items))
	for _, s := range list.Items {
		res, err := c.api.ContainerInspect(ctx, s.ID, client.ContainerInspectOptions{})
		if err != nil {
			// The container was removed between list and inspect.
			if cerrdefs.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("inspect container %s: %w", ShortID(s.ID), c.wrapErr(err))
		}
		containers = append(containers, newContainer(s, res.Container))
	}

	return containers, nil
}

// sortByName orders containers by name in place.
func sortByName(cs []Container) {
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Name < cs[j].Name
	})
}

// ShortID returns the first 12 characters of a container ID, like the docker CLI.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
