package regions

import (
	"context"
	"fmt"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCmdRegions() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "regions",
		Aliases: []string{"rg"},
		Short:   "Manage regions",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdCreate())

	return cmd
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List regions in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			resp, err := client.GetV1RegionsWithResponse(context.Background())
			if err != nil {
				return fmt.Errorf("fetching regions: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No regions found in this workspace.")
				return nil
			}

			header := []string{"ID", "NAME", "ICON"}
			var rows [][]string
			for _, r := range resp.JSON200.Data {
				icon := "-"
				if r.Icon != nil {
					icon = *r.Icon
				}
				rows = append(rows, []string{r.Id, r.Name, icon})
			}

			output.PrintTable(header, rows)
			return nil
		},
	}
}

func newCmdCreate() *cobra.Command {
	var name, icon string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a region",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			body := api.PostV1RegionsJSONRequestBody{
				Name: name,
			}
			if icon != "" {
				body.Icon = &icon
			}

			resp, err := client.PostV1RegionsWithResponse(context.Background(), body)
			if err != nil {
				return fmt.Errorf("creating region: %w", err)
			}
			if resp.JSON201 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403, resp.JSON422)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON201.Data)
			}

			fmt.Printf("Region %s created (%s).\n", resp.JSON201.Data.Name, resp.JSON201.Data.Id)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Region name (required)")
	cmd.Flags().StringVar(&icon, "icon", "", "Icon URL")
	_ = cmd.MarkFlagRequired("name")

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
