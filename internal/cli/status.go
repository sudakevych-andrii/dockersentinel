package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
	"github.com/sudakevych-andrii/dockersentinel/internal/monitor"
	"github.com/sudakevych-andrii/dockersentinel/internal/projects"
)

type statusOptions struct {
	projects []string
	json     bool
}

// statusReport is the JSON output of the status command.
// Field names are part of the CLI contract and must stay stable.
type statusReport struct {
	Docker   dockerReport    `json:"docker"`
	Projects []projectStatus `json:"projects"`
}

type dockerReport struct {
	Host       string `json:"host"`
	APIVersion string `json:"api_version"`
	OSType     string `json:"os_type"`
}

type projectStatus struct {
	Name       string `json:"name"`
	Standalone bool   `json:"standalone"`
	Containers int    `json:"containers"`
	Running    int    `json:"running"`
	Unhealthy  int    `json:"unhealthy"`
}

func newStatusCmd(gopts *options) *cobra.Command {
	opts := &statusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the summary state of all projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), cmd.OutOrStdout(), gopts, opts)
		},
	}

	cmd.Flags().StringArrayVar(&opts.projects, "project", nil, "show only this project (repeatable)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "output in JSON format")

	return cmd
}

func runStatus(ctx context.Context, w io.Writer, gopts *options, opts *statusOptions) error {
	cfg, _, err := loadConfig(gopts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, dockerTimeout)
	defer cancel()

	dc, info, err := connect(ctx, cfg.Docker.Host)
	if err != nil {
		return err
	}
	defer dc.Close()

	containers, err := dc.ListContainers(ctx)
	if err != nil {
		return err
	}

	containers = monitor.NewFilter(cfg).Apply(containers)

	selected, missing := projects.Select(projects.Group(containers), opts.projects)
	if len(missing) > 0 {
		return fmt.Errorf("project not found: %s", strings.Join(missing, ", "))
	}

	report := statusReport{
		Docker: dockerReport{
			Host:       info.Host,
			APIVersion: info.APIVersion,
			OSType:     info.OSType,
		},
		Projects: make([]projectStatus, 0, len(selected)),
	}
	for _, p := range selected {
		report.Projects = append(report.Projects, newProjectStatus(p))
	}

	if opts.json {
		return writeJSON(w, report)
	}
	return writeStatusText(w, report)
}

func newProjectStatus(p projects.Project) projectStatus {
	s := projectStatus{
		Name:       p.Name,
		Standalone: p.Standalone,
		Containers: len(p.Containers),
	}
	for _, c := range p.Containers {
		if c.State == docker.StateRunning {
			s.Running++
		}
		if c.Health == docker.HealthUnhealthy {
			s.Unhealthy++
		}
	}
	return s
}

func writeStatusText(w io.Writer, r statusReport) error {
	fmt.Fprintf(w, "Docker connected: %s (API %s, %s)\n\n", r.Docker.Host, r.Docker.APIVersion, r.Docker.OSType)

	if len(r.Projects) == 0 {
		_, err := fmt.Fprintln(w, "No containers found.")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tCONTAINERS\tRUNNING\tUNHEALTHY")
	for _, p := range r.Projects {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", p.Name, p.Containers, p.Running, p.Unhealthy)
	}
	return tw.Flush()
}
