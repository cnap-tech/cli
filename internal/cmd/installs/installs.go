package installs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewCmdInstalls() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "installs",
		Aliases: []string{"inst"},
		Short:   "Manage installs",
	}

	cmd.AddCommand(newCmdList())
	cmd.AddCommand(newCmdGet())
	cmd.AddCommand(newCmdCreate())
	cmd.AddCommand(newCmdDelete())
	cmd.AddCommand(newCmdUpdateValues())
	cmd.AddCommand(newCmdUpdateOverrides())
	cmd.AddCommand(newCmdPods())
	cmd.AddCommand(newCmdLogs())

	return cmd
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installs in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			resp, err := client.GetV1InstallsWithResponse(context.Background())
			if err != nil {
				return fmt.Errorf("fetching installs: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No installs found in this workspace.")
				return nil
			}

			header := []string{"ID", "NAME", "PRODUCT", "CLUSTER", "CREATED"}
			var rows [][]string
			for _, i := range resp.JSON200.Data {
				name := "-"
				if i.Name != nil {
					name = *i.Name
				}
				productId := "-"
				if i.ProductId != nil {
					productId = *i.ProductId
				}
				rows = append(rows, []string{i.Id, name, productId, i.ClusterId, formatTime(i.CreatedAt)})
			}

			output.PrintTable(header, rows)
			return nil
		},
	}
}

func newCmdGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get <install-id>",
		Short: "Get install details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.GetV1InstallsIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("fetching install: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			i := resp.JSON200.Data

			output.PrintTable(
				[]string{"FIELD", "VALUE"},
				[][]string{
					{"ID", i.Id},
					{"Name", deref(i.Name)},
					{"Workspace", i.WorkspaceId},
					{"Product", deref(i.ProductId)},
					{"Template", deref(i.TemplateId)},
					{"Cluster", i.ClusterId},
				},
			)
			return nil
		},
	}
}

func newCmdDelete() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <install-id>",
		Short: "Delete an install",
		Long:  "Triggers an async deletion workflow that removes the ArgoCD application and install record.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("this will permanently delete the install. Use --force to confirm")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.DeleteV1InstallsIdWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("deleting install: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 202 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			fmt.Printf("Install %s deletion started.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Confirm deletion")

	return cmd
}

func newCmdCreate() *cobra.Command {
	var productID, regionID string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a product install",
		Long:  "Deploys a product to a region. Starts an async workflow.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			body := api.PostV1InstallsJSONRequestBody{
				ProductId: productID,
				RegionId:  regionID,
			}

			resp, err := client.PostV1InstallsWithResponse(context.Background(), body)
			if err != nil {
				return fmt.Errorf("creating install: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 202 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403, resp.JSON422)
			}

			fmt.Println("Install workflow started.")
			return nil
		},
	}

	cmd.Flags().StringVar(&productID, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&regionID, "region", "", "Region ID (required)")
	_ = cmd.MarkFlagRequired("product")
	_ = cmd.MarkFlagRequired("region")

	return cmd
}

func newCmdUpdateValues() *cobra.Command {
	var sourceID, valuesFile string

	cmd := &cobra.Command{
		Use:   "update-values <install-id>",
		Short: "Update install template values",
		Long:  "Updates template helm source values and regenerates the chart.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			values, err := readValuesFile(valuesFile)
			if err != nil {
				return err
			}

			body := api.PatchV1InstallsIdValuesJSONRequestBody{
				Updates: []struct {
					TemplateHelmSourceId string                  `json:"templateHelmSourceId"`
					Values               map[string]*interface{} `json:"values"`
				}{
					{
						TemplateHelmSourceId: sourceID,
						Values:               values,
					},
				},
			}

			resp, err := client.PatchV1InstallsIdValuesWithResponse(context.Background(), args[0], body)
			if err != nil {
				return fmt.Errorf("updating install values: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 202 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404, resp.JSON422)
			}

			fmt.Println("Install values update started.")
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "Helm source ID (required)")
	cmd.Flags().StringVarP(&valuesFile, "values", "f", "", "Values YAML/JSON file (required)")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("values")

	return cmd
}

func newCmdUpdateOverrides() *cobra.Command {
	var sourceID, valuesFile string

	cmd := &cobra.Command{
		Use:   "update-overrides <install-id>",
		Short: "Update install value overrides",
		Long:  "Applies per-install value overrides on top of product base values.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			values, err := readValuesFile(valuesFile)
			if err != nil {
				return err
			}

			body := api.PatchV1InstallsIdOverridesJSONRequestBody{
				Updates: []struct {
					TemplateHelmSourceId string                  `json:"templateHelmSourceId"`
					Values               map[string]*interface{} `json:"values"`
				}{
					{
						TemplateHelmSourceId: sourceID,
						Values:               values,
					},
				},
			}

			resp, err := client.PatchV1InstallsIdOverridesWithResponse(context.Background(), args[0], body)
			if err != nil {
				return fmt.Errorf("updating install overrides: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 202 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404, resp.JSON422)
			}

			fmt.Println("Install overrides update started.")
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "Helm source ID (required)")
	cmd.Flags().StringVarP(&valuesFile, "values", "f", "", "Values YAML/JSON file (required)")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("values")

	return cmd
}

func readValuesFile(path string) (map[string]*interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading values file: %w", err)
	}

	var raw map[string]interface{}

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, &raw); err != nil {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parsing values file (expected JSON or YAML): %w", err)
		}
	}

	// Convert to map[string]*interface{} for the API client
	result := make(map[string]*interface{}, len(raw))
	for k, v := range raw {
		val := v
		result[k] = &val
	}
	return result, nil
}

func newCmdPods() *cobra.Command {
	return &cobra.Command{
		Use:   "pods <install-id>",
		Short: "List pods for an install",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			resp, err := client.GetV1InstallsIdPodsWithResponse(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("fetching pods: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200.Data)
			}

			if len(resp.JSON200.Data) == 0 {
				fmt.Println("No pods found for this install.")
				return nil
			}

			header := []string{"POD", "CONTAINERS"}
			var rows [][]string
			for _, p := range resp.JSON200.Data {
				rows = append(rows, []string{p.Name, strings.Join(p.Containers, ", ")})
			}

			output.PrintTable(header, rows)
			return nil
		},
	}
}

func newCmdLogs() *cobra.Command {
	var pod, container string
	var follow bool
	var tail, sinceSeconds int

	cmd := &cobra.Command{
		Use:   "logs <install-id>",
		Short: "Stream logs from an install",
		Long:  "Streams logs from install pods via Server-Sent Events.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			params := &api.GetV1InstallsIdLogsParams{
				Follow: &follow,
			}
			if pod != "" {
				params.Pod = &pod
			}
			if container != "" {
				params.Container = &container
			}
			if tail > 0 {
				params.Tail = &tail
			}
			if sinceSeconds > 0 {
				params.SinceSeconds = &sinceSeconds
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			// Use raw client to get streaming response
			resp, err := client.GetV1InstallsIdLogs(ctx, args[0], params)
			if err != nil {
				return fmt.Errorf("streaming logs: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				return fmt.Errorf("unexpected response: %s", resp.Status)
			}

			// Read SSE stream line by line
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				// SSE format: "data: <log line>"
				if strings.HasPrefix(line, "data: ") {
					fmt.Println(line[6:])
				}
			}

			return scanner.Err()
		},
	}

	cmd.Flags().StringVar(&pod, "pod", "", "Pod name (all pods if omitted)")
	cmd.Flags().StringVar(&container, "container", "", "Container name")
	cmd.Flags().BoolVarP(&follow, "follow", "f", true, "Follow log output")
	cmd.Flags().IntVar(&tail, "tail", 0, "Number of lines to tail")
	cmd.Flags().IntVar(&sinceSeconds, "since", 0, "Only return logs newer than this many seconds")

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

func deref(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func formatTime(ts float32) string {
	return fmt.Sprintf("%.0f", ts)
}
