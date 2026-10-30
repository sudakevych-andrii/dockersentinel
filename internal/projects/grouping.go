package projects

import (
	"sort"

	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
)

// Docker Compose labels used for grouping.
const (
	LabelProject = "com.docker.compose.project"
	LabelService = "com.docker.compose.service"
)

// StandaloneName is the display name of the group for containers without Compose labels.
const StandaloneName = "Standalone"

// Project is a group of containers: a Compose project or the Standalone group.
type Project struct {
	Name       string             `json:"name"`
	Standalone bool               `json:"standalone"`
	Containers []docker.Container `json:"containers"`
}

// ProjectName returns the Compose project of the container.
// ok is false for standalone containers.
func ProjectName(c docker.Container) (name string, ok bool) {
	name = c.Labels[LabelProject]
	return name, name != ""
}

// ServiceName returns the Compose service name, or the container name for standalone containers.
func ServiceName(c docker.Container) string {
	if s := c.Labels[LabelService]; s != "" {
		return s
	}
	return c.Name
}

// Group splits containers into Compose projects sorted by name.
// Standalone containers go to a separate group, which is always last.
func Group(containers []docker.Container) []Project {
	byName := make(map[string]*Project)
	standalone := &Project{Name: StandaloneName, Standalone: true}

	for _, c := range containers {
		name, ok := ProjectName(c)
		if !ok {
			standalone.Containers = append(standalone.Containers, c)
			continue
		}

		p, exists := byName[name]
		if !exists {
			p = &Project{Name: name}
			byName[name] = p
		}
		p.Containers = append(p.Containers, c)
	}

	result := make([]Project, 0, len(byName)+1)
	for _, p := range byName {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	if len(standalone.Containers) > 0 {
		result = append(result, *standalone)
	}

	for i := range result {
		sortContainers(result[i].Containers)
	}

	return result
}

// sortContainers orders containers by service name, then by container name for replicas.
func sortContainers(cs []docker.Container) {
	sort.Slice(cs, func(i, j int) bool {
		si, sj := ServiceName(cs[i]), ServiceName(cs[j])
		if si != sj {
			return si < sj
		}
		return cs[i].Name < cs[j].Name
	})
}

// Select returns only the projects with the given names, keeping their order.
// Empty names means all projects. missing lists names that matched no project.
func Select(ps []Project, names []string) (selected []Project, missing []string) {
	if len(names) == 0 {
		return ps, nil
	}

	found := make(map[string]bool, len(names))
	for _, n := range names {
		found[n] = false
	}

	selected = make([]Project, 0, len(names))
	for _, p := range ps {
		if _, ok := found[p.Name]; ok {
			selected = append(selected, p)
			found[p.Name] = true
		}
	}

	for _, n := range names {
		if !found[n] {
			missing = append(missing, n)
			found[n] = true // report each missing name once
		}
	}

	return selected, missing
}
