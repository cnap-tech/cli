package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/useragent"
	"github.com/spf13/cobra"
)

func NewCmdAuth() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	cmd.AddCommand(newCmdLogin())
	cmd.AddCommand(newCmdLogout())
	cmd.AddCommand(newCmdStatus())

	return cmd
}

func newCmdLogin() *cobra.Command {
	var token string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with CNAP",
		Long: `Authenticate via browser (default) or with a token.

Without flags, opens your browser to authenticate via the device flow
and stores a session token. Sessions are long-lived and auto-refresh on use.

With --token, stores the given token directly (PAT or session token).

Create PATs at https://dash.cnap.tech/settings/tokens`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if token != "" {
				cfg.Auth.Token = token
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("saving config: %w", err)
				}
				fmt.Println("Logged in successfully. Token saved to ~/.cnap/config.yaml")
				return nil
			}

			return runDeviceFlow(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVarP(&token, "token", "t", "", "API token (PAT or session token)")

	return cmd
}

func newCmdLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Revoke session server-side if it's a session token
			token := cfg.Token()
			if token != "" && !strings.HasPrefix(token, "cnap_pat_") && !strings.HasPrefix(token, "eyJ") {
				if err := revokeSession(cmd.Context(), cfg, token); err != nil {
					slog.Debug("failed to revoke session server-side", "error", err)
				}
			}

			cfg.Auth.Token = ""
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Println("Logged out. Credentials removed from ~/.cnap/config.yaml")
			return nil
		},
	}
}

func newCmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			token := cfg.Token()
			if token == "" {
				fmt.Println("Not authenticated. Run: cnap auth login")
				return nil
			}

			tokenType := detectTokenType(token)

			// Show prefix only for security
			prefix := token
			if len(prefix) > 16 {
				prefix = prefix[:16] + "..."
			}

			fmt.Printf("Token type: %s\n", tokenType)
			fmt.Printf("Token: %s\n", prefix)
			fmt.Printf("API URL: %s\n", cfg.BaseURL())
			fmt.Printf("Auth URL: %s\n", cfg.AuthBaseURL())

			if tokenType == "Session token" {
				if err := checkSessionStatus(cmd.Context(), cfg, token); err != nil {
					fmt.Printf("Session status: invalid or expired (%v)\n", err)
					fmt.Println("Run 'cnap auth login' to re-authenticate.")
				}
			}

			if cfg.ActiveWorkspace != "" {
				fmt.Printf("Active workspace: %s\n", cfg.ActiveWorkspace)
			} else {
				fmt.Println("No active workspace. Run: cnap workspaces switch <id>")
			}

			return nil
		},
	}
}

func detectTokenType(token string) string {
	switch {
	case strings.HasPrefix(token, "cnap_pat_"):
		return "Personal Access Token (PAT)"
	case strings.HasPrefix(token, "eyJ"):
		return "JWT"
	default:
		return "Session token"
	}
}

func checkSessionStatus(ctx context.Context, cfg *config.Config, token string) error {
	authURL := cfg.AuthBaseURL()
	req, err := http.NewRequestWithContext(ctx, "GET", authURL+"/api/auth/get-session", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", useragent.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Session *struct {
			ExpiresAt string `json:"expiresAt"`
		} `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Session == nil {
		return fmt.Errorf("session not found or expired")
	}

	fmt.Printf("Session status: active (expires: %s)\n", result.Session.ExpiresAt)
	return nil
}

func revokeSession(ctx context.Context, cfg *config.Config, token string) error {
	authURL := cfg.AuthBaseURL()
	req, err := http.NewRequestWithContext(ctx, "POST", authURL+"/api/auth/sign-out", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", useragent.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	return nil
}
