package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	authcmd "github.com/cnap-tech/cli/internal/cmd/auth"
	clusterscmd "github.com/cnap-tech/cli/internal/cmd/clusters"
	installscmd "github.com/cnap-tech/cli/internal/cmd/installs"
	productscmd "github.com/cnap-tech/cli/internal/cmd/products"
	regionscmd "github.com/cnap-tech/cli/internal/cmd/regions"
	registrycmd "github.com/cnap-tech/cli/internal/cmd/registry"
	templatescmd "github.com/cnap-tech/cli/internal/cmd/templates"
	workspacescmd "github.com/cnap-tech/cli/internal/cmd/workspaces"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/debug"
	"github.com/cnap-tech/cli/internal/update"
	"github.com/cnap-tech/cli/internal/useragent"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
)

func Execute(ctx context.Context) error {
	root := rootCmd()

	// Background update check (gh CLI pattern)
	updateCh := make(chan *update.ReleaseInfo)
	go func() {
		if version == "dev" || !update.ShouldCheckForUpdate() {
			updateCh <- nil
			return
		}
		checkCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rel, _ := update.CheckForUpdate(checkCtx, version)
		updateCh <- rel
	}()

	err := root.ExecuteContext(ctx)

	// Print update notice after command output
	if newRelease := <-updateCh; newRelease != nil {
		isHomebrew := update.IsUnderHomebrew()
		if !(isHomebrew && update.IsRecentRelease(newRelease.PublishedAt)) {
			fmt.Fprintf(os.Stderr, "\nA new release of cnap is available: %s → %s\n",
				strings.TrimPrefix(version, "v"),
				strings.TrimPrefix(newRelease.Version, "v"))
			if isHomebrew {
				fmt.Fprintf(os.Stderr, "To upgrade, run: brew upgrade cnap\n")
			}
			fmt.Fprintf(os.Stderr, "%s\n", newRelease.URL)
		}
	}

	return err
}

func rootCmd() *cobra.Command {
	useragent.SetVersion(version)

	var debugFlag bool

	root := &cobra.Command{
		Use:   "cnap",
		Short: "CNAP CLI — manage workspaces, clusters, and deployments",
		Long: `CNAP CLI provides programmatic access to your CNAP workspace.

Manage clusters, templates, products, and deployments from the terminal.
Authenticate with a Personal Access Token or via browser login.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       fmt.Sprintf("%s (%s)", version, commit),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			debug.Init(debugFlag)
			if debug.Enabled {
				debug.Install()
			}
		},
	}

	root.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Enable debug logging (or set CNAP_DEBUG=1)")
	root.PersistentFlags().StringVarP(&cmdutil.OutputFormat, "output", "o", "", "Output format: table, json, quiet")
	root.PersistentFlags().StringVar(&cmdutil.APIURL, "api-url", "", "API base URL (overrides config)")

	root.AddCommand(authcmd.NewCmdAuth())
	root.AddCommand(workspacescmd.NewCmdWorkspaces())
	root.AddCommand(clusterscmd.NewCmdClusters())
	root.AddCommand(templatescmd.NewCmdTemplates())
	root.AddCommand(productscmd.NewCmdProducts())
	root.AddCommand(installscmd.NewCmdInstalls())
	root.AddCommand(regionscmd.NewCmdRegions())
	root.AddCommand(registrycmd.NewCmdRegistry())

	return root
}
