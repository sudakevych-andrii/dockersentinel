package cli

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
)

// dockerTimeout limits a single CLI request to the Docker Engine.
const dockerTimeout = 10 * time.Second

// options holds global flags shared by all commands.
type options struct {
	host string
}

// NewRootCmd builds the root command with all subcommands.
func NewRootCmd(version string) *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:           "dockersentinel",
		Short:         "Monitor containers of a local Docker Engine",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&opts.host, "host", "",
		"Docker host (default: DOCKER_HOST or unix:///var/run/docker.sock)")

	cmd.AddCommand(newStatusCmd(opts))
	cmd.AddCommand(newWatchCmd(opts))

	return cmd
}

// connect creates a Docker client and checks that the Engine is reachable.
// The caller must close the returned client.
func connect(ctx context.Context, host string) (*docker.Client, docker.EngineInfo, error) {
	dc, err := docker.NewClient(host)
	if err != nil {
		return nil, docker.EngineInfo{}, err
	}

	info, err := dc.Ping(ctx)
	if err != nil {
		dc.Close()
		return nil, docker.EngineInfo{}, err
	}

	return dc, info, nil
}

// writeJSON writes v as indented JSON. It is used by all commands with --json.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
