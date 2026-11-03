package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/sudakevych-andrii/dockersentinel/internal/docker"
	"github.com/sudakevych-andrii/dockersentinel/internal/monitor"
)

func newWatchCmd(gopts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Run the long-lived monitoring process",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWatch(cmd.Context(), cmd.ErrOrStderr(), gopts)
		},
	}
}

func runWatch(ctx context.Context, logOut io.Writer, gopts *options) error {
	cfg, path, err := loadConfig(gopts)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: cfg.LogLevel()}))

	dc, err := docker.NewClient(cfg.Docker.Host)
	if err != nil {
		return err
	}
	defer dc.Close()

	// Missing permissions will not fix themselves, so fail fast.
	// An unavailable daemon is fine: discovery keeps reconnecting.
	pingCtx, cancel := context.WithTimeout(ctx, dockerTimeout)
	_, err = dc.Ping(pingCtx)
	cancel()
	if errors.Is(err, docker.ErrPermissionDenied) {
		return err
	}

	log.Info("watch started", "config", displayPath(path), "mode", cfg.Monitoring.Mode)
	monitor.NewDiscovery(dc, docker.NewInventory(), monitor.NewFilter(cfg), log).Run(ctx)
	log.Info("watch stopped")

	return nil
}
