package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/sudakevych-andrii/dockersentinel/internal/config"
	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
)

// dockerTimeout limits a single CLI request to the Docker Engine.
const dockerTimeout = 10 * time.Second

// options holds global flags shared by all commands.
type options struct {
	configPath string
	host       string
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

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "",
		"config file (default: ./"+config.FileName+" or ~/.config/dockersentinel/config.yaml)")
	cmd.PersistentFlags().StringVar(&opts.host, "host", "",
		"Docker host, overrides docker.host (default: DOCKER_HOST or unix:///var/run/docker.sock)")

	cmd.AddCommand(newStatusCmd(opts))
	cmd.AddCommand(newWatchCmd(opts))
	cmd.AddCommand(newConfigCmd(opts))

	return cmd
}

// loadConfig finds, loads and validates the configuration.
// path is empty when no config file was found and the defaults are used.
// The --host flag overrides docker.host.
func loadConfig(opts *options) (cfg config.Config, path string, err error) {
	cfg = config.Default()

	path = config.Locate(opts.configPath)
	if path != "" {
		if cfg, err = config.Load(path); err != nil {
			return config.Config{}, path, err
		}
	}

	if err := cfg.Validate(); err != nil {
		return config.Config{}, path, fmt.Errorf("invalid config %s:\n%w", displayPath(path), err)
	}

	if opts.host != "" {
		cfg.Docker.Host = opts.host
	}
	return cfg, path, nil
}

// displayPath names the config source in messages.
func displayPath(path string) string {
	if path == "" {
		return "(defaults)"
	}
	return path
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
