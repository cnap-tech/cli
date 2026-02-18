package clusters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/cnap-tech/cli/internal/prompt"
	"github.com/spf13/cobra"
)

func NewCmdClusters() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusters",
		Aliases: []string{"cluster", "cl"},
		Short:   "Manage clusters",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdGet())
	cmd.AddCommand(newCmdUpdate())
	cmd.AddCommand(newCmdDelete())
	cmd.AddCommand(newCmdKubeconfig())

	return cmd
}

func newCmdList() *cobra.Command {
	var limit int
	var cursor string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List clusters in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			params := &api.GetV1ClustersParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1ClustersWithResponse(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("fetching clusters: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			header := []string{"ID", "NAME", "REGION", "TYPE", "STATUS"}
			var rows [][]string
			for _, c := range resp.JSON200.Data {
				clusterType := "imported"
				status := "-"
				if c.Kaas != nil {
					clusterType = "kaas"
					status = string(c.Kaas.Status)
				}
				rows = append(rows, []string{c.Id, c.Name, c.RegionId, clusterType, status})
			}

			if len(rows) == 0 {
				fmt.Println("No clusters found in this workspace.")
				return nil
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
		Use:   "get [cluster-id]",
		Short: "Get cluster details",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<cluster-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			clusterID := ""
			if len(args) > 0 {
				clusterID = args[0]
			} else {
				clusterID, err = pickCluster(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1ClustersIdWithResponse(cmd.Context(), clusterID)
			if err != nil {
				return fmt.Errorf("fetching cluster: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			c := resp.JSON200
			clusterType := "imported"
			status := "-"
			if c.Kaas != nil {
				clusterType = "kaas"
				status = string(c.Kaas.Status)
				if c.Kaas.StatusMessage != nil {
					status += " (" + *c.Kaas.StatusMessage + ")"
				}
			}

			output.PrintTable(
				[]string{"FIELD", "VALUE"},
				[][]string{
					{"ID", c.Id},
					{"Name", c.Name},
					{"Workspace", c.WorkspaceId},
					{"Region", c.RegionId},
					{"Type", clusterType},
					{"Status", status},
				},
			)
			return nil
		},
	}
}

func newCmdUpdate() *cobra.Command {
	var name, regionID string

	cmd := &cobra.Command{
		Use:   "update [cluster-id]",
		Short: "Update cluster name or region",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<cluster-id> argument required when not running interactively")
			}

			if name == "" && regionID == "" {
				return fmt.Errorf("at least one of --name or --region is required")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			clusterID := ""
			if len(args) > 0 {
				clusterID = args[0]
			} else {
				clusterID, err = pickCluster(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			body := api.PatchV1ClustersIdJSONRequestBody{}
			if name != "" {
				body.Name = &name
			}
			if regionID != "" {
				body.RegionId = &regionID
			}

			resp, err := client.PatchV1ClustersIdWithResponse(cmd.Context(), clusterID, body)
			if err != nil {
				return fmt.Errorf("updating cluster: %w", err)
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response: %s", resp.Status())
			}

			fmt.Printf("Cluster %s updated.\n", resp.JSON200.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New cluster name")
	cmd.Flags().StringVar(&regionID, "region", "", "New region ID")

	return cmd
}

func newCmdDelete() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [cluster-id]",
		Short: "Delete a cluster",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<cluster-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			clusterID := ""
			if len(args) > 0 {
				clusterID = args[0]
			} else {
				clusterID, err = pickCluster(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			if !yes {
				if !prompt.IsInteractive() {
					return fmt.Errorf("use --yes to confirm deletion in non-interactive mode")
				}
				confirmed, err := prompt.Confirm(fmt.Sprintf("Delete cluster %s?", clusterID))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteV1ClustersIdWithResponse(cmd.Context(), clusterID)
			if err != nil {
				return fmt.Errorf("deleting cluster: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return fmt.Errorf("unexpected response: %s", resp.Status())
			}

			fmt.Printf("Cluster %s deleted.\n", clusterID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func newCmdKubeconfig() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "kubeconfig [cluster-id]",
		Short: "Get cluster admin kubeconfig",
		Long:  "Downloads the admin kubeconfig for a KaaS-managed cluster. The cluster must be running.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<cluster-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			clusterID := ""
			if len(args) > 0 {
				clusterID = args[0]
			} else {
				clusterID, err = pickCluster(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1ClustersIdKubeconfig(cmd.Context(), clusterID)
			if err != nil {
				return fmt.Errorf("fetching kubeconfig: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if resp.StatusCode != 200 {
				var apiErr api.Error
				if json.Unmarshal(body, &apiErr) == nil {
					return fmt.Errorf("%s", apiErr.Error.Message)
				}
				return fmt.Errorf("unexpected response: %s", resp.Status)
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, body, 0600); err != nil {
					return fmt.Errorf("writing kubeconfig: %w", err)
				}
				fmt.Printf("Kubeconfig written to %s\n", outputFile)
				return nil
			}

			fmt.Print(string(body))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write kubeconfig to file (mode 0600)")

	return cmd
}

// pickCluster shows an interactive cluster picker. Returns the selected cluster ID.
func pickCluster(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	limit := 100
	listResp, err := client.GetV1ClustersWithResponse(ctx, &api.GetV1ClustersParams{Limit: &limit})
	if err != nil {
		return "", fmt.Errorf("fetching clusters: %w", err)
	}
	if listResp.JSON200 == nil {
		return "", apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
	}
	if len(listResp.JSON200.Data) == 0 {
		return "", fmt.Errorf("no clusters found in this workspace")
	}
	options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
	for i, c := range listResp.JSON200.Data {
		options[i] = prompt.SelectOption{Label: c.Name + " (" + c.Id + ")", Value: c.Id}
	}
	return prompt.Select("Select a cluster", options)
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
