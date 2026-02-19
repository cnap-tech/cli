package installs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/cnap-tech/cli/internal/prompt"
	"github.com/cnap-tech/cli/internal/useragent"
	"github.com/coder/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

func NewCmdInstalls() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "installs",
		Aliases: []string{"install", "inst"},
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
	cmd.AddCommand(newCmdExec())

	return cmd
}

func newCmdList() *cobra.Command {
	var limit int
	var cursor string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installs in the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			if cfg.ActiveWorkspace == "" {
				return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
			}

			params := &api.GetV1InstallsParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}

			resp, err := client.GetV1InstallsWithResponse(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("fetching installs: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON403)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
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
		Use:   "get [install-id]",
		Short: "Get install details",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			installID := ""
			if len(args) > 0 {
				installID = args[0]
			} else {
				installID, err = pickInstall(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1InstallsIdWithResponse(cmd.Context(), installID)
			if err != nil {
				return fmt.Errorf("fetching install: %w", err)
			}
			if resp.JSON200 == nil {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			format := cmdutil.GetOutputFormat(cfg)
			if format == output.FormatJSON {
				return output.PrintJSON(resp.JSON200)
			}

			i := resp.JSON200

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
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [install-id]",
		Short: "Delete an install",
		Long:  "Triggers an async deletion workflow that removes the ArgoCD application and install record.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			installID := ""
			if len(args) > 0 {
				installID = args[0]
			} else {
				installID, err = pickInstall(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			if !yes {
				if !prompt.IsInteractive() {
					return fmt.Errorf("use --yes to confirm deletion in non-interactive mode")
				}
				confirmed, err := prompt.Confirm(fmt.Sprintf("Delete install %s?", installID))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteV1InstallsIdWithResponse(cmd.Context(), installID)
			if err != nil {
				return fmt.Errorf("deleting install: %w", err)
			}
			if resp.HTTPResponse.StatusCode != 202 {
				return apiError(resp.Status(), resp.JSON401, resp.JSON404)
			}

			fmt.Printf("Install %s deletion started.\n", installID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

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

			resp, err := client.PostV1InstallsWithResponse(cmd.Context(), nil, body)
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
		Use:   "update-values [install-id]",
		Short: "Update install template values",
		Long:  "Updates template helm source values and regenerates the chart.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			installID := ""
			if len(args) > 0 {
				installID = args[0]
			} else {
				installID, err = pickInstall(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			values, err := readValuesFile(valuesFile)
			if err != nil {
				return err
			}

			body := api.PatchV1InstallsIdValuesJSONRequestBody{
				Updates: []struct {
					TemplateHelmSourceId string                  `json:"template_helm_source_id"`
					Values               map[string]*interface{} `json:"values"`
				}{
					{
						TemplateHelmSourceId: sourceID,
						Values:               values,
					},
				},
			}

			resp, err := client.PatchV1InstallsIdValuesWithResponse(cmd.Context(), installID, body)
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
		Use:   "update-overrides [install-id]",
		Short: "Update install value overrides",
		Long:  "Applies per-install value overrides on top of product base values.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, _, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			installID := ""
			if len(args) > 0 {
				installID = args[0]
			} else {
				installID, err = pickInstall(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			values, err := readValuesFile(valuesFile)
			if err != nil {
				return err
			}

			body := api.PatchV1InstallsIdOverridesJSONRequestBody{
				Updates: []struct {
					TemplateHelmSourceId string                  `json:"template_helm_source_id"`
					Values               map[string]*interface{} `json:"values"`
				}{
					{
						TemplateHelmSourceId: sourceID,
						Values:               values,
					},
				},
			}

			resp, err := client.PatchV1InstallsIdOverridesWithResponse(cmd.Context(), installID, body)
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
		Use:   "pods [install-id]",
		Short: "List pods for an install",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			installID := ""
			if len(args) > 0 {
				installID = args[0]
			} else {
				installID, err = pickInstall(cmd.Context(), client)
				if err != nil {
					return err
				}
			}

			resp, err := client.GetV1InstallsIdPodsWithResponse(cmd.Context(), installID)
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
		Use:   "logs [install-id]",
		Short: "Stream logs from an install",
		Long: `Streams logs from install pods via Server-Sent Events.

When run interactively without arguments, shows pickers to select an
install, pod, and container. In non-interactive environments (CI, pipes),
the install ID argument is required.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			var installID string

			if len(args) > 0 {
				installID = args[0]
			} else {
				// Interactive install picker
				if cfg.ActiveWorkspace == "" {
					return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
				}
				limit := 100
				listResp, err := client.GetV1InstallsWithResponse(cmd.Context(), &api.GetV1InstallsParams{Limit: &limit})
				if err != nil {
					return fmt.Errorf("fetching installs: %w", err)
				}
				if listResp.JSON200 == nil {
					return apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
				}
				if len(listResp.JSON200.Data) == 0 {
					return fmt.Errorf("no installs found in this workspace")
				}

				options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
				for i, inst := range listResp.JSON200.Data {
					label := inst.Id
					if inst.Name != nil {
						label = *inst.Name + " (" + inst.Id + ")"
					}
					options[i] = prompt.SelectOption{Label: label, Value: inst.Id}
				}

				installID, err = prompt.Select("Select an install", options)
				if err != nil {
					return err
				}
			}

			// Interactive pod picker if --pod not set
			if pod == "" && prompt.IsInteractive() {
				podsResp, err := client.GetV1InstallsIdPodsWithResponse(cmd.Context(), installID)
				if err != nil {
					return fmt.Errorf("fetching pods: %w", err)
				}
				if podsResp.JSON200 != nil && len(podsResp.JSON200.Data) > 0 {
					podOpts := make([]prompt.SelectOption, len(podsResp.JSON200.Data))
					for i, p := range podsResp.JSON200.Data {
						podOpts[i] = prompt.SelectOption{
							Label: p.Name + " [" + strings.Join(p.Containers, ", ") + "]",
							Value: p.Name,
						}
					}

					pod, err = prompt.Select("Select a pod", podOpts)
					if err != nil {
						return err
					}

					// Interactive container picker if pod has multiple containers
					if container == "" {
						for _, p := range podsResp.JSON200.Data {
							if p.Name == pod && len(p.Containers) > 1 {
								containerOpts := make([]prompt.SelectOption, len(p.Containers))
								for i, c := range p.Containers {
									containerOpts[i] = prompt.SelectOption{Label: c, Value: c}
								}
								container, err = prompt.Select("Select a container", containerOpts)
								if err != nil {
									return err
								}
								break
							}
						}
					}
				}
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

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()

			// Use raw client to get streaming response
			resp, err := client.GetV1InstallsIdLogs(ctx, installID, params)
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

func newCmdExec() *cobra.Command {
	var pod, container, shell string

	cmd := &cobra.Command{
		Use:   "exec [install-id]",
		Short: "Open an interactive shell in a pod container",
		Long: `Opens a WebSocket connection to a pod container for interactive shell access.

When run interactively without arguments, shows pickers to select an
install, pod, and container. In non-interactive environments, all
arguments and flags are required.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !prompt.IsInteractive() {
				return fmt.Errorf("<install-id> argument required when not running interactively")
			}

			client, cfg, err := cmdutil.NewClient()
			if err != nil {
				return err
			}

			var installID string

			if len(args) > 0 {
				installID = args[0]
			} else {
				// Interactive install picker
				if cfg.ActiveWorkspace == "" {
					return fmt.Errorf("no active workspace. Run: cnap workspaces switch <id>")
				}
				limit := 100
				listResp, err := client.GetV1InstallsWithResponse(cmd.Context(), &api.GetV1InstallsParams{Limit: &limit})
				if err != nil {
					return fmt.Errorf("fetching installs: %w", err)
				}
				if listResp.JSON200 == nil {
					return apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
				}
				if len(listResp.JSON200.Data) == 0 {
					return fmt.Errorf("no installs found in this workspace")
				}

				options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
				for i, inst := range listResp.JSON200.Data {
					label := inst.Id
					if inst.Name != nil {
						label = *inst.Name + " (" + inst.Id + ")"
					}
					options[i] = prompt.SelectOption{Label: label, Value: inst.Id}
				}

				installID, err = prompt.Select("Select an install", options)
				if err != nil {
					return err
				}
			}

			// Interactive pod picker if --pod not set
			if pod == "" && prompt.IsInteractive() {
				podsResp, err := client.GetV1InstallsIdPodsWithResponse(cmd.Context(), installID)
				if err != nil {
					return fmt.Errorf("fetching pods: %w", err)
				}
				if podsResp.JSON200 != nil && len(podsResp.JSON200.Data) > 0 {
					podOpts := make([]prompt.SelectOption, len(podsResp.JSON200.Data))
					for i, p := range podsResp.JSON200.Data {
						podOpts[i] = prompt.SelectOption{
							Label: p.Name + " [" + strings.Join(p.Containers, ", ") + "]",
							Value: p.Name,
						}
					}

					pod, err = prompt.Select("Select a pod", podOpts)
					if err != nil {
						return err
					}

					// Interactive container picker if pod has multiple containers
					if container == "" {
						for _, p := range podsResp.JSON200.Data {
							if p.Name == pod {
								if len(p.Containers) > 1 {
									containerOpts := make([]prompt.SelectOption, len(p.Containers))
									for i, c := range p.Containers {
										containerOpts[i] = prompt.SelectOption{Label: c, Value: c}
									}
									container, err = prompt.Select("Select a container", containerOpts)
									if err != nil {
										return err
									}
								} else if len(p.Containers) == 1 {
									container = p.Containers[0]
								}
								break
							}
						}
					}
				}
			}

			if pod == "" || container == "" {
				return fmt.Errorf("--pod and --container are required")
			}

			return runExec(cmd.Context(), cfg, installID, pod, container, shell)
		},
	}

	cmd.Flags().StringVar(&pod, "pod", "", "Pod name")
	cmd.Flags().StringVar(&container, "container", "", "Container name")
	cmd.Flags().StringVar(&shell, "shell", "/bin/sh", "Shell to use")

	return cmd
}

// runExec connects to the WebSocket exec endpoint and bridges it to the local terminal.
func runExec(parentCtx context.Context, cfg *config.Config, installID, podName, containerName, shell string) error {
	// Build WebSocket URL from the dashboard/auth URL (where exec handler lives)
	baseURL := cfg.AuthBaseURL()
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parsing auth URL: %w", err)
	}

	// Convert http(s) to ws(s)
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/api/exec/installs/%s/shell", installID)
	q := u.Query()
	q.Set("podName", podName)
	q.Set("containerName", containerName)
	q.Set("shell", shell)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// Connect
	conn, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + cfg.Token()},
			"User-Agent":    []string{useragent.String()},
		},
	})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("WebSocket connection failed (HTTP %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()

	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("stdin is not a terminal")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("setting raw terminal mode: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	// Send initial terminal size
	sendResize(ctx, conn)

	// Handle SIGWINCH (terminal resize) and SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	done := make(chan struct{})

	// Goroutine: read from WebSocket → write to stdout
	go func() {
		defer close(done)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg wsMessage
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "output":
				_, _ = os.Stdout.Write([]byte(msg.Data))
			case "error":
				_, _ = fmt.Fprintf(os.Stderr, "\r\nError: %s\r\n", msg.Message)
			case "close":
				return
			}
		}
	}()

	// Goroutine: read from stdin → send to WebSocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			msg, _ := json.Marshal(wsMessage{Type: "input", Data: string(buf[:n])})
			if conn.Write(ctx, websocket.MessageText, msg) != nil {
				return
			}
		}
	}()

	// Goroutine: handle signals
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGWINCH:
				sendResize(ctx, conn)
			case syscall.SIGINT, syscall.SIGTERM:
				_ = conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}()

	<-done
	return nil
}

type wsMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

func sendResize(ctx context.Context, conn *websocket.Conn) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}
	msg, _ := json.Marshal(wsMessage{Type: "resize", Cols: w, Rows: h})
	_ = conn.Write(ctx, websocket.MessageText, msg)
}

// pickInstall shows an interactive install picker. Returns the selected install ID.
func pickInstall(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	limit := 100
	listResp, err := client.GetV1InstallsWithResponse(ctx, &api.GetV1InstallsParams{Limit: &limit})
	if err != nil {
		return "", fmt.Errorf("fetching installs: %w", err)
	}
	if listResp.JSON200 == nil {
		return "", apiError(listResp.Status(), listResp.JSON401, listResp.JSON403)
	}
	if len(listResp.JSON200.Data) == 0 {
		return "", fmt.Errorf("no installs found in this workspace")
	}
	options := make([]prompt.SelectOption, len(listResp.JSON200.Data))
	for i, inst := range listResp.JSON200.Data {
		label := inst.Id
		if inst.Name != nil {
			label = *inst.Name + " (" + inst.Id + ")"
		}
		options[i] = prompt.SelectOption{Label: label, Value: inst.Id}
	}
	return prompt.Select("Select an install", options)
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
