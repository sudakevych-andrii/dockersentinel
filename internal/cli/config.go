package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newConfigCmd(gopts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Work with the configuration file",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Check the configuration file and report all problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigValidate(cmd.OutOrStdout(), gopts)
		},
	})

	return cmd
}

func runConfigValidate(w io.Writer, gopts *options) error {
	_, path, err := loadConfig(gopts)
	if err != nil {
		return err
	}

	if path == "" {
		_, err = fmt.Fprintln(w, "No config file found, using defaults: OK")
		return err
	}
	_, err = fmt.Fprintf(w, "%s: OK\n", path)
	return err
}
