package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/cnap-tech/cli/internal/prompt"
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
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registry credentials in the active workspace",
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

			resp, err := client.GetV1RegistryCredentialsWithResponse(cmd.Context(), params)
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
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [credential-id]",
		Short: "Delete a registry credential",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<credential-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			credentialID := ""
			if len(args) > 0 {
				credentialID = args[0]
			} else {
				credentialID, err = pickCredential(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			if !yes {
				if !prompt.IsInteractive() {
					return fmt.Errorf("use --yes to confirm deletion in non-interactive mode")
				}
				confirmed, err := prompt.Confirm(fmt.Sprintf("Delete registry credential %s?", credentialID))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteV1RegistryCredentialsIdWithResponse(cmd.Context(), credentialID)
			if err != nil {
				return fmt.Errorf("deleting credential: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			fmt.Printf("Registry credential %s deleted.\n", credentialID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

// pickCredential shows an interactive registry credential picker. Returns the selected credential ID.
func pickCredential(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	limit := 100
	listResp, err := client.GetV1RegistryCredentialsWithResponse(ctx, &api.GetV1RegistryCredentialsParams{Limit: &limit})
	if err != nil {
		return "", fmt.Errorf("fetching registry credentials: %w", err)
	}
	if listResp.JSON200 == nil {
		return "", apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
	}
	if len(listResp.JSON200.Data) == 0 {
		return "", fmt.Errorf("no registry credentials found in this workspace")
	}
	options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
	for i, c := range listResp.JSON200.Data {
		options[i] = prompt.SelectOption{Label: c.Name + " (" + c.RegistryUrl + ")", Value: c.Id}
	}
	return prompt.Select("Select a credential", options)
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
