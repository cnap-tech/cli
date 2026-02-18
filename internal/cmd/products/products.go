package products

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

func NewCmdProducts() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "products",
		Aliases: []string{"product", "prod"},
		Short:   "Manage products",
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
		Short:   "List products in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			params := &api.GetV1ProductsParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1ProductsWithResponse(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("fetching products: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
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
		Use:   "get [product-id]",
		Short: "Get product details",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<product-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			productID := ""
			if len(args) > 0 {
				productID = args[0]
			} else {
				productID, err = pickProduct(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1ProductsIdWithResponse(cmd.Context(), productID)
			if err != nil {
				return fmt.Errorf("fetching product: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			p := resp.JSON200

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
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [product-id]",
		Short: "Delete a product",
		Long:  "Delete a product. Fails if the product has active installs.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<product-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			productID := ""
			if len(args) > 0 {
				productID = args[0]
			} else {
				productID, err = pickProduct(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			if !yes {
				if !prompt.IsInteractive() {
					return fmt.Errorf("use --yes to confirm deletion in non-interactive mode")
				}
				confirmed, err := prompt.Confirm(fmt.Sprintf("Delete product %s?", productID))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteV1ProductsIdWithResponse(cmd.Context(), productID)
			if err != nil {
				return fmt.Errorf("deleting product: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404, resp.JSON409)
			}

			fmt.Printf("Product %s deleted.\n", productID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

// pickProduct shows an interactive product picker. Returns the selected product ID.
func pickProduct(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	limit := 100
	listResp, err := client.GetV1ProductsWithResponse(ctx, &api.GetV1ProductsParams{Limit: &limit})
	if err != nil {
		return "", fmt.Errorf("fetching products: %w", err)
	}
	if listResp.JSON200 == nil {
		return "", apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
	}
	if len(listResp.JSON200.Data) == 0 {
		return "", fmt.Errorf("no products found in this workspace")
	}
	options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
	for i, p := range listResp.JSON200.Data {
		options[i] = prompt.SelectOption{Label: p.Name + " (" + p.Id + ")", Value: p.Id}
	}
	return prompt.Select("Select a product", options)
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
