package auth

import (
	"fmt"

	"github.com/cnap-tech/cli/internal/config"
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
		Long: `Authenticate via browser (default) or with a Personal Access Token.

Without flags, opens your browser to authenticate via the device flow.
With --token, stores the given Personal Access Token directly.

Create tokens at https://dash.cnap.tech/settings/tokens`,
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

	cmd.Flags().StringVarP(&token, "token", "t", "", "Personal Access Token (cnap_pat_...)")

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

			cfg.Auth.Token = ""
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Println("Logged out. Token removed from ~/.cnap/config.yaml")
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
				fmt.Println("Not authenticated. Run: cnap auth login --token <your-token>")
				return nil
			}

			// Show prefix only for security
			prefix := token
			if len(prefix) > 16 {
				prefix = prefix[:16] + "..."
			}
			fmt.Printf("Authenticated with token: %s\n", prefix)
			fmt.Printf("API URL: %s\n", cfg.BaseURL())
			fmt.Printf("Auth URL: %s\n", cfg.AuthBaseURL())

			if cfg.ActiveWorkspace != "" {
				fmt.Printf("Active workspace: %s\n", cfg.ActiveWorkspace)
			} else {
				fmt.Println("No active workspace. Run: cnap workspaces switch <id>")
			}

			return nil
		},
	}
}
