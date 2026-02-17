package workspaces

import (
	"fmt"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCmdWorkspaces() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspaces",
		Aliases: []string{"ws"},
		Short:   "Manage workspaces",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdSwitch())

	return cmd
}

func newCmdList() *cobra.Command {
	var limit int
	var cursor string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			params := &api.GetV1WorkspacesParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1WorkspacesWithResponse(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("fetching workspaces: %w", err)
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response: %s", resp.Status())
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			header := []string{"ID", "NAME"}
			var rows [][]string
			for _, w := range resp.JSON200.Data {
				active := ""
				if w.Id == cfg.ActiveWorkspace {
					active = " (active)"
				}
				rows = append(rows, []string{w.Id, w.Name + active})
			}
			output.PrintTable(header, rows)
			if resp.JSON200.Pagination.HasMore {
				fmt.Printf("\nMore results available. Use --cursor %s to see next page.\n", *resp.JSON200.Pagination.Cursor)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Items per page (1-100)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor from previous response")

	return cmd
}

func newCmdSwitch() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <workspace-id>",
		Short: "Set the active workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cfg.ActiveWorkspace = args[0]
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("Active workspace set to: %s\n", args[0])
			return nil
		},
	}
}
