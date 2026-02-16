package cmd

import (
	"fmt"

	authcmd "github.com/cnap-tech/cli/internal/cmd/auth"
	clusterscmd "github.com/cnap-tech/cli/internal/cmd/clusters"
	workspacescmd "github.com/cnap-tech/cli/internal/cmd/workspaces"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
)

func Execute() error {
	return rootCmd().Execute()
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cnap",
		Short: "CNAP CLI — manage workspaces, clusters, and deployments",
		Long: `CNAP CLI provides programmatic access to your CNAP workspace.

Manage clusters, templates, products, and deployments from the terminal.
Authenticate with a Personal Access Token or via browser login.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       fmt.Sprintf("%s (%s)", version, commit),
	}

	root.PersistentFlags().StringVarP(&cmdutil.OutputFormat, "output", "o", "", "Output format: table, json, quiet")
	root.PersistentFlags().StringVar(&cmdutil.APIURL, "api-url", "", "API base URL (overrides config)")

	root.AddCommand(authcmd.NewCmdAuth())
	root.AddCommand(workspacescmd.NewCmdWorkspaces())
	root.AddCommand(clusterscmd.NewCmdClusters())

	return root
}
