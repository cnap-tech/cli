package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCmdRegistry() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"reg"},
		Short:   "Manage registry credentials",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdDelete())

	return cmd
}

func newCmdList() *cobra.Command {
	var limit int
	var cursor string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registry credentials in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			params := &api.GetV1RegistryCredentialsParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1RegistryCredentialsWithResponse(context.Background(), params)
			if err != nil {
				return fmt.Errorf("fetching registry credentials: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No registry credentials found in this workspace.")
				return nil
			}

			header := []string{"ID", "NAME", "REGISTRY", "TYPE", "ACTIVE"}
			var rows [][]string
			for _, c := range resp.JSON200.Data {
				active := "yes"
				if !c.IsActive {
					active = "no"
				}
				rows = append(rows, []string{c.Id, c.Name, c.RegistryUrl, string(c.Type), active})
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

func newCmdDelete() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <credential-id>",
		Short: "Delete a registry credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("this will permanently delete the credential. Use --force to confirm")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.DeleteV1RegistryCredentialsIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("deleting credential: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			fmt.Printf("Registry credential %s deleted.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Confirm deletion")

	return cmd
}

func apiError(status string, errs ...*api.Error) error {
	for _, e := range errs {
		if e != nil {
			parts := []string{e.Error.Message}
			if e.Error.Suggestion != nil {
				parts = append(parts, *e.Error.Suggestion)
			}
			return fmt.Errorf("%s", strings.Join(parts, ". "))
		}
	}
	return fmt.Errorf("unexpected response: %s", status)
}
