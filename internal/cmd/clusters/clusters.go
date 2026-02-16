package clusters

import (
	"context"
	"fmt"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCmdClusters() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusters",
		Aliases: []string{"cl"},
		Short:   "Manage clusters",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdGet())
	cmd.AddCommand(newCmdUpdate())
	cmd.AddCommand(newCmdDelete())

	return cmd
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List clusters in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			resp, err := client.GetV1ClustersWithResponse(context.Background())
			if err != nil {
				return fmt.Errorf("fetching clusters: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
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
			return nil
		},
	}
}

func newCmdGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get <cluster-id>",
		Short: "Get cluster details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.GetV1ClustersIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("fetching cluster: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			c := resp.JSON200.Data
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
		Use:   "update <cluster-id>",
		Short: "Update cluster name or region",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" && regionID == "" {
				return fmt.Errorf("at least one of --name or --region is required")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			body := api.PatchV1ClustersIdJSONRequestBody{}
			if name != "" {
				body.Name = &name
			}
			if regionID != "" {
				body.RegionId = &regionID
			}

			resp, err := client.PatchV1ClustersIdWithResponse(context.Background(), args[0], body)
			if err != nil {
				return fmt.Errorf("updating cluster: %w", err)
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("unexpected response: %s", resp.Status())
			}

			fmt.Printf("Cluster %s updated.\n", resp.JSON200.Data.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New cluster name")
	cmd.Flags().StringVar(&regionID, "region", "", "New region ID")

	return cmd
}

func newCmdDelete() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <cluster-id>",
		Short: "Delete a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("this will permanently delete the cluster. Use --force to confirm")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.DeleteV1ClustersIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("deleting cluster: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 204 {
				return fmt.Errorf("unexpected response: %s", resp.Status())
			}

			fmt.Printf("Cluster %s deleted.\n", args[0])
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
