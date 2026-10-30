package docker

import (
	"fmt"
	"sync"
	"testing"
)

func names(cs []Container) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestInventoryReplace(t *testing.T) {
	inv := NewInventory()

	added, removed := inv.Replace([]Container{{ID: "b", Name: "web"}, {ID: "a", Name: "api"}})
	if got := fmt.Sprint(names(added)); got != "[api web]" || len(removed) != 0 {
		t.Fatalf("first resync: added=%s removed=%v", got, names(removed))
	}

	// "web" is gone, "db" is new, "api" stays.
	added, removed = inv.Replace([]Container{{ID: "a", Name: "api"}, {ID: "c", Name: "db"}})
	if got := fmt.Sprint(names(added)); got != "[db]" {
		t.Errorf("added = %s, want [db]", got)
	}
	if got := fmt.Sprint(names(removed)); got != "[web]" {
		t.Errorf("removed = %s, want [web]", got)
	}
	if got := fmt.Sprint(names(inv.List())); got != "[api db]" {
		t.Errorf("list = %s, want [api db]", got)
	}
}

func TestInventoryUpsertRemove(t *testing.T) {
	inv := NewInventory()

	if _, existed := inv.Upsert(Container{ID: "a", Name: "api", State: StateRunning}); existed {
		t.Error("first upsert: existed = true")
	}

	prev, existed := inv.Upsert(Container{ID: "a", Name: "api", State: StateExited})
	if !existed || prev.State != StateRunning {
		t.Errorf("second upsert: existed=%v prev.State=%s, want true, running", existed, prev.State)
	}
	if c, _ := inv.Get("a"); c.State != StateExited {
		t.Errorf("state = %s, want exited", c.State)
	}

	if c, ok := inv.Remove("a"); !ok || c.Name != "api" {
		t.Errorf("remove: ok=%v c=%+v", ok, c)
	}
	if _, ok := inv.Remove("a"); ok {
		t.Error("second remove: ok = true")
	}
	if _, ok := inv.Get("a"); ok {
		t.Error("container still present after remove")
	}
}

// TestInventoryConcurrent is meaningful with -race.
func TestInventoryConcurrent(t *testing.T) {
	inv := NewInventory()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprint(i)
			inv.Upsert(Container{ID: id, Name: id})
			inv.List()
			inv.Remove(id)
		}()
	}
	wg.Wait()

	if n := len(inv.List()); n != 0 {
		t.Errorf("len = %d, want 0", n)
	}
}
