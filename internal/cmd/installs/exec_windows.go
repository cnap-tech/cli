package installs

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCmdExec() *cobra.Command {
	return &cobra.Command{
		Use:   "exec [install-id]",
		Short: "Open an interactive shell in a pod container",
		Long:  "The exec command is not supported on Windows.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("exec is not supported on Windows")
		},
	}
}
