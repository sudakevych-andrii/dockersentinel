package projects

import (
	"testing"

	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
)

func compose(id, name, project, service string) docker.Container {
	return docker.Container{
		ID:   id,
		Name: name,
		Labels: map[string]string{
			LabelProject: project,
			LabelService: service,
		},
	}
}

func standalone(id, name string) docker.Container {
	return docker.Container{ID: id, Name: name, Labels: map[string]string{}}
}

func TestGroup(t *testing.T) {
	containers := []docker.Container{
		standalone("s1", "hello"),
		compose("w2", "shop-web-2", "shop", "web"),
		compose("a1", "blog-app-1", "blog", "app"),
		compose("w1", "shop-web-1", "shop", "web"),
		compose("d1", "shop-db-1", "shop", "db"),
	}

	got := Group(containers)

	want := []struct {
		name       string
		standalone bool
		ids        []string
	}{
		{"blog", false, []string{"a1"}},
		{"shop", false, []string{"d1", "w1", "w2"}},
		{StandaloneName, true, []string{"s1"}},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d projects, want %d", len(got), len(want))
	}

	for i, w := range want {
		p := got[i]
		if p.Name != w.name || p.Standalone != w.standalone {
			t.Errorf("project[%d] = %q (standalone=%v), want %q (standalone=%v)",
				i, p.Name, p.Standalone, w.name, w.standalone)
			continue
		}
		if len(p.Containers) != len(w.ids) {
			t.Errorf("%s: got %d containers, want %d", p.Name, len(p.Containers), len(w.ids))
			continue
		}
		for j, id := range w.ids {
			if p.Containers[j].ID != id {
				t.Errorf("%s: container[%d] = %s, want %s", p.Name, j, p.Containers[j].ID, id)
			}
		}
	}
}

func TestGroupWithoutStandalone(t *testing.T) {
	got := Group([]docker.Container{compose("a1", "blog-app-1", "blog", "app")})

	if len(got) != 1 || got[0].Name != "blog" {
		t.Fatalf("got %+v, want only project blog", got)
	}
}

func TestGroupEmpty(t *testing.T) {
	if got := Group(nil); len(got) != 0 {
		t.Fatalf("got %d projects, want 0", len(got))
	}
}

func TestServiceName(t *testing.T) {
	if got := ServiceName(compose("x", "shop-web-1", "shop", "web")); got != "web" {
		t.Errorf("compose: got %q, want web", got)
	}
	if got := ServiceName(standalone("x", "hello")); got != "hello" {
		t.Errorf("standalone: got %q, want hello", got)
	}
}

func TestSelect(t *testing.T) {
	ps := []Project{{Name: "blog"}, {Name: "shop"}, {Name: StandaloneName, Standalone: true}}

	got, missing := Select(ps, []string{StandaloneName, "nope", "shop", "nope"})

	if len(got) != 2 || got[0].Name != "shop" || got[1].Name != StandaloneName {
		t.Errorf("selected = %+v, want shop, Standalone in original order", got)
	}
	if len(missing) != 1 || missing[0] != "nope" {
		t.Errorf("missing = %v, want [nope]", missing)
	}
}

func TestSelectAll(t *testing.T) {
	ps := []Project{{Name: "blog"}, {Name: "shop"}}

	got, missing := Select(ps, nil)

	if len(got) != 2 || missing != nil {
		t.Errorf("got %+v, missing %v; want all projects and no missing", got, missing)
	}
}
