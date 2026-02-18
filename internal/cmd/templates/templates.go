package templates

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

func NewCmdTemplates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Aliases: []string{"template", "tpl"},
		Short:   "Manage templates",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdGet())
	cmd.AddCommand(newCmdDelete())

	return cmd
}

func newCmdList() *cobra.Command {
	var limit int
	var cursor string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List templates in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			params := &api.GetV1TemplatesParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1TemplatesWithResponse(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("fetching templates: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No templates found in this workspace.")
				return nil
			}

			header := []string{"ID", "NAME", "PROXY MODE", "CREATED"}
			var rows [][]string
			for _, t := range resp.JSON200.Data {
				proxyMode := "-"
				if t.RegistryProxyMode != nil {
					proxyMode = string(*t.RegistryProxyMode)
				}
				rows = append(rows, []string{t.Id, t.Name, proxyMode, formatTime(t.CreatedAt)})
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

func newCmdGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get [template-id]",
		Short: "Get template details with helm sources",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<template-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			templateID := ""
			if len(args) > 0 {
				templateID = args[0]
			} else {
				templateID, err = pickTemplate(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1TemplatesIdWithResponse(cmd.Context(), templateID)
			if err != nil {
				return fmt.Errorf("fetching template: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			t := resp.JSON200
			proxyMode := "-"
			if t.RegistryProxyMode != nil {
				proxyMode = string(*t.RegistryProxyMode)
			}

			output.PrintTable(
				[]string{"FIELD", "VALUE"},
				[][]string{
					{"ID", t.Id},
					{"Name", t.Name},
					{"Workspace", t.WorkspaceId},
					{"Registry Proxy", proxyMode},
					{"Sources", fmt.Sprintf("%d helm source(s)", len(t.HelmSources))},
				},
			)

			if len(t.HelmSources) > 0 {
				fmt.Println()
				header := []string{"SOURCE ID", "REPO URL", "CHART", "VERSION"}
				var rows [][]string
				for _, s := range t.HelmSources {
					chart := deref(s.Chart.Chart)
					if chart == "" && s.Chart.Path != nil {
						chart = *s.Chart.Path
					}
					rows = append(rows, []string{s.Id, s.Chart.RepoUrl, chart, s.Chart.TargetRevision})
				}
				output.PrintTable(header, rows)
			}

			return nil
		},
	}
}

func newCmdDelete() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [template-id]",
		Short: "Delete a template",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<template-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			templateID := ""
			if len(args) > 0 {
				templateID = args[0]
			} else {
				templateID, err = pickTemplate(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			if !yes {
				if !prompt.IsInteractive() {
					return fmt.Errorf("use --yes to confirm deletion in non-interactive mode")
				}
				confirmed, err := prompt.Confirm(fmt.Sprintf("Delete template %s?", templateID))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteV1TemplatesIdWithResponse(cmd.Context(), templateID)
			if err != nil {
				return fmt.Errorf("deleting template: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			fmt.Printf("Template %s deleted.\n", templateID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

// pickTemplate shows an interactive template picker. Returns the selected template ID.
func pickTemplate(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	limit := 100
	listResp, err := client.GetV1TemplatesWithResponse(ctx, &api.GetV1TemplatesParams{Limit: &limit})
	if err != nil {
		return "", fmt.Errorf("fetching templates: %w", err)
	}
	if listResp.JSON200 == nil {
		return "", apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
	}
	if len(listResp.JSON200.Data) == 0 {
		return "", fmt.Errorf("no templates found in this workspace")
	}
	options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
	for i, t := range listResp.JSON200.Data {
		options[i] = prompt.SelectOption{Label: t.Name + " (" + t.Id + ")", Value: t.Id}
	}
	return prompt.Select("Select a template", options)
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

func deref(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func formatTime(ts float32) string {
	return fmt.Sprintf("%.0f", ts)
}
