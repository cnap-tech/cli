package cmdutil

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cnap-tech/cli/internal/api"
	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/output"
	"github.com/cnap-tech/cli/internal/useragent"
)

// OutputFormat holds the CLI-level --output flag value.
// Set by the root command's PersistentFlags.
var OutputFormat string

// APIURL holds the CLI-level --api-url flag value.
var APIURL string

// NewClient creates an authenticated API client from config.
func NewClient() (*api.ClientWithResponses, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	if APIURL != "" {
		cfg.APIURL = APIURL
	}

	token := cfg.Token()
	if token == "" {
		return nil, nil, fmt.Errorf("not authenticated. Run: cnap auth login --token <your-token>")
	}

	baseURL := cfg.BaseURL()

	client, err := api.NewClientWithResponses(baseURL, api.WithRequestEditorFn(
		func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("User-Agent", useragent.String())
			if cfg.ActiveWorkspace != "" {
				req.Header.Set("X-Workspace-Id", cfg.ActiveWorkspace)
			}
			return nil
		},
	))
	if err != nil {
		return nil, nil, fmt.Errorf("creating API client: %w", err)
	}

	return client, cfg, nil
}

// GetOutputFormat returns the effective output format.
func GetOutputFormat(cfg *config.Config) output.Format {
	if OutputFormat != "" {
		return output.Format(OutputFormat)
	}
	if cfg.Output.Format != "" {
		return output.Format(cfg.Output.Format)
	}
	return output.FormatTable
}
