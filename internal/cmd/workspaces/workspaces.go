package workspaces

import (
	"fmt"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/cnap-tech/cli/internal/prompt"
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
		Use:   "switch [workspace-id]",
		Short: "Set the active workspace",
		Long: `Set the active workspace for subsequent commands.

When run interactively without arguments, shows a picker to select a workspace.
In non-interactive environments (CI, pipes), the workspace ID argument is required.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Fail fast in non-interactive mode without an argument
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<workspace-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			var workspaceID string

			if len(args) > 0 {
				// Validate the workspace ID by fetching it
				workspaceID = args[0]
				resp, err := client.GetV1WorkspacesIdWithResponse(cmd.Context(), workspaceID)
				if err != nil {
					return fmt.Errorf("validating workspace: %w", err)
				}
				if resp.JSON200 == nil {
					return fmt.Errorf("workspace %q not found", workspaceID)
				}
				fmt.Printf("Workspace: %s\n", resp.JSON200.Name)
			} else {
				// Fetch workspaces for interactive selection
				limit := 100
				params := &api.GetV1WorkspacesParams{Limit: &limit}
				resp, err := client.GetV1WorkspacesWithResponse(cmd.Context(), params)
				if err != nil {
					return fmt.Errorf("fetching workspaces: %w", err)
				}
				if resp.JSON200 == nil {
					return fmt.Errorf("unexpected response: %s", resp.Status())
				}

				if len(resp.JSON200.Data) == 0 {
					return fmt.Errorf("no workspaces found")
				}

				options := make([]prompt.SelectOption, len(resp.JSON200.Data))
				for i, w := range resp.JSON200.Data {
					label := w.Name
					if w.Id == cfg.ActiveWorkspace {
						label += " (active)"
					}
					options[i] = prompt.SelectOption{Label: label, Value: w.Id}
				}

				workspaceID, err = prompt.Select("Select a workspace", options)
				if err != nil {
					return err
				}
			}

			cfg.ActiveWorkspace = workspaceID
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("Active workspace set to: %s\n", workspaceID)
			return nil
		},
	}
}
