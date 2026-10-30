package docker

import "sync"

// Inventory is a thread-safe in-memory snapshot of all containers, keyed by container ID.
// It is filled by a full resync and kept up to date by Docker events.
// Returned containers share labels and ports with the inventory, so treat them as read-only.
type Inventory struct {
	mu         sync.RWMutex
	containers map[string]Container
}

// NewInventory returns an empty inventory.
func NewInventory() *Inventory {
	return &Inventory{containers: make(map[string]Container)}
}

// Replace sets the full list of containers after a resync and reports what changed.
// Both results are sorted by name.
func (inv *Inventory) Replace(cs []Container) (added, removed []Container) {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	next := make(map[string]Container, len(cs))
	for _, c := range cs {
		next[c.ID] = c
		if _, ok := inv.containers[c.ID]; !ok {
			added = append(added, c)
		}
	}
	for id, c := range inv.containers {
		if _, ok := next[id]; !ok {
			removed = append(removed, c)
		}
	}
	inv.containers = next

	sortByName(added)
	sortByName(removed)
	return added, removed
}

// Upsert adds or updates a container. It returns the previous version, if any.
func (inv *Inventory) Upsert(c Container) (prev Container, existed bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	prev, existed = inv.containers[c.ID]
	inv.containers[c.ID] = c
	return prev, existed
}

// Remove deletes a container. ok is false if it was not in the inventory.
func (inv *Inventory) Remove(id string) (c Container, ok bool) {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	c, ok = inv.containers[id]
	delete(inv.containers, id)
	return c, ok
}

// Get returns a container by ID.
func (inv *Inventory) Get(id string) (Container, bool) {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	c, ok := inv.containers[id]
	return c, ok
}

// List returns all containers sorted by name.
func (inv *Inventory) List() []Container {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	cs := make([]Container, 0, len(inv.containers))
	for _, c := range inv.containers {
		cs = append(cs, c)
	}
	sortByName(cs)
	return cs
}
