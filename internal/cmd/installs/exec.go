package installs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/cnap-tech/cli/internal/cmdutil"
	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/prompt"
	"github.com/cnap-tech/cli/internal/useragent"
	"github.com/coder/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
				installID, err = pickInstall(cmd.Context(), client)
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

	// Start platform-specific resize monitoring (SIGWINCH on Unix, polling on Windows)
	resizeStop := make(chan struct{})
	go monitorResize(ctx, conn, resizeStop)

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

	// Goroutine: handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case <-sigCh:
			_ = conn.Close(websocket.StatusNormalClosure, "")
		case <-done:
		}
		close(resizeStop)
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
