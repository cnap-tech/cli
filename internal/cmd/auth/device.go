// Device authorization flow (RFC 8628) for browser-based CLI authentication.
//
// Flow:
//  1. POST /api/auth/device/code           - device_code + user_code
//  2. Open browser to /device?user_code=X  - user approves
//  3. POST /api/auth/device/token (poll)   - session token (access_token)
//  4. Store session token in ~/.cnap/config.yaml
//
// The session token is sent as a Bearer token in subsequent API requests.
// The public API middleware validates it via BetterAuth's get-session endpoint.
// Sessions are valid for 1 year and auto-refresh on use, so active users never expire.
//
// For CI/CD, PATs (cnap_pat_...) are still supported via --token flag.
//
// See also: public-api.middleware.ts (server-side token verification)
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/cnap-tech/cli/internal/config"
	"github.com/cnap-tech/cli/internal/useragent"
)

const clientID = "cnap-cli"

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type deviceTokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func runDeviceFlow(ctx context.Context, cfg *config.Config) error {
	authURL := cfg.AuthBaseURL()
	slog.Debug("starting device flow", "auth_url", authURL, "api_url", cfg.BaseURL())

	// Step 1: Request device code
	code, err := requestDeviceCode(ctx, authURL)
	if err != nil {
		return fmt.Errorf("requesting device code: %w", err)
	}

	// Step 2: Display instructions and open browser
	verificationURL := fmt.Sprintf("%s/device?user_code=%s", authURL, code.UserCode)
	fmt.Printf("\nTo authenticate, open this URL in your browser:\n\n")
	fmt.Printf("  %s\n\n", verificationURL)
	fmt.Printf("And verify this code: %s\n\n", formatUserCode(code.UserCode))

	if err := openBrowser(verificationURL); err != nil {
		fmt.Println("(Could not open browser automatically)")
	} else {
		fmt.Println("Browser opened. Waiting for authorization...")
	}

	// Step 3: Poll for session token
	interval := time.Duration(code.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)

	sessionToken, err := pollForToken(ctx, authURL, code.DeviceCode, interval, deadline)
	if err != nil {
		return err
	}

	// Step 4: Store session token directly
	cfg.Auth.Token = sessionToken
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("\nLogged in successfully. Session token saved to ~/.cnap/config.yaml")
	return nil
}

func requestDeviceCode(ctx context.Context, authURL string) (*deviceCodeResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": clientID,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", authURL+"/api/auth/device/code", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", useragent.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func pollForToken(ctx context.Context, authURL, deviceCode string, interval time.Duration, deadline time.Time) (string, error) {
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device authorization expired — please try again")
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		body, _ := json.Marshal(map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": deviceCode,
			"client_id":   clientID,
		})

		req, err := http.NewRequestWithContext(ctx, "POST", authURL+"/api/auth/device/token", bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("polling for token: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", useragent.String())

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("polling for token: %w", err)
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck

		if resp.StatusCode == 200 {
			var tokenResp deviceTokenResponse
			if err := json.Unmarshal(data, &tokenResp); err != nil {
				return "", fmt.Errorf("parsing token response: %w", err)
			}
			return tokenResp.AccessToken, nil
		}

		var errResp deviceTokenError
		if err := json.Unmarshal(data, &errResp); err != nil {
			return "", fmt.Errorf("parsing error response: %w", err)
		}

		switch errResp.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "expired_token":
			return "", fmt.Errorf("device code expired — please try again")
		case "access_denied":
			return "", fmt.Errorf("authorization was denied")
		default:
			return "", fmt.Errorf("unexpected error: %s — %s", errResp.Error, errResp.ErrorDescription)
		}
	}
}

func formatUserCode(code string) string {
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
