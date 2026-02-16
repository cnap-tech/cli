package products

import (
	"context"
	"fmt"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCmdProducts() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "products",
		Aliases: []string{"prod"},
		Short:   "Manage products",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdGet())
	cmd.AddCommand(newCmdDelete())

	return cmd
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List products in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			resp, err := client.GetV1ProductsWithResponse(context.Background())
			if err != nil {
				return fmt.Errorf("fetching products: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No products found in this workspace.")
				return nil
			}

			header := []string{"ID", "NAME", "TEMPLATE", "CREATED"}
			var rows [][]string
			for _, p := range resp.JSON200.Data {
				rows = append(rows, []string{p.Id, p.Name, p.TemplateId, formatTime(p.CreatedAt)})
			}

			output.PrintTable(header, rows)
			return nil
		},
	}
}

func newCmdGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get <product-id>",
		Short: "Get product details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.GetV1ProductsIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("fetching product: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			p := resp.JSON200.Data

			output.PrintTable(
				[]string{"FIELD", "VALUE"},
				[][]string{
					{"ID", p.Id},
					{"Name", p.Name},
					{"Workspace", p.WorkspaceId},
					{"Template", p.TemplateId},
				},
			)
			return nil
		},
	}
}

func newCmdDelete() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <product-id>",
		Short: "Delete a product",
		Long:  "Delete a product. Fails if the product has active installs.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("this will permanently delete the product. Use --force to confirm")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.DeleteV1ProductsIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("deleting product: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404, resp.JSON409)
			}

			fmt.Printf("Product %s deleted.\n", args[0])
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

func formatTime(ts float32) string {
	return fmt.Sprintf("%.0f", ts)
}
