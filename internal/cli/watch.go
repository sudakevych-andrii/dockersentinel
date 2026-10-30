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
	log := slog.New(slog.NewTextHandler(logOut, nil))

	dc, err := docker.NewClient(gopts.host)
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

	log.Info("watch started")
	monitor.NewDiscovery(dc, docker.NewInventory(), log).Run(ctx)
	log.Info("watch stopped")

	return nil
}
