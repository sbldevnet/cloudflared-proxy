package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = ""
)

func Version() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s %s\n", version, commit)
			return err
		},
	}

	return cmd
}
