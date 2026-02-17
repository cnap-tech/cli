// Device authorization flow (RFC 8628) for browser-based CLI authentication.
//
// Flow:
//  1. POST /api/auth/device/code           - device_code + user_code
//  2. Open browser to /device?user_code=X  - user approves
//  3. POST /api/auth/device/token (poll)   - session token (access_token)
//  4. GET  /api/auth/convex/token          - Convex JWT (Bearer session token)
//  5. POST /v1/user/tokens                 - PAT (Bearer Convex JWT)
//  6. Store PAT in ~/.cnap/config.yaml     - all subsequent CLI requests use PAT
//
// Why exchange session for Convex JWT then PAT?
//   - The device flow returns an opaque session token (not a JWT)
//   - The public API middleware only accepts JWTs or PATs, not session tokens
//   - BetterAuth's convex plugin provides /api/auth/convex/token which converts
//     a session token (via Bearer header) into a Convex JWT
//   - The PAT is stored permanently; the session/JWT are discarded after bootstrap
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

type jwtTokenResponse struct {
	Token string `json:"token"`
}

type createdTokenResponse struct {
	Data struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	} `json:"data"`
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

	// Step 3: Poll for token
	interval := time.Duration(code.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)

	sessionToken, err := pollForToken(ctx, authURL, code.DeviceCode, interval, deadline)
	if err != nil {
		return err
	}

	slog.Debug("device flow authorized, exchanging tokens")
	fmt.Println("\nAuthorized! Creating API token...")

	// Step 4: Exchange session token for a JWT (the public API middleware verifies JWTs, not session tokens)
	jwtToken, err := exchangeSessionForJWT(ctx, authURL, sessionToken)
	if err != nil {
		return fmt.Errorf("exchanging session for JWT: %w", err)
	}

	// Step 5: Use JWT to create a PAT via the public API
	pat, err := createPATFromJWT(ctx, cfg, jwtToken)
	if err != nil {
		return fmt.Errorf("creating API token: %w", err)
	}

	// Step 6: Store PAT in config
	cfg.Auth.Token = pat
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Logged in successfully. Token saved to ~/.cnap/config.yaml")
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

// exchangeSessionForJWT calls BetterAuth's /api/auth/convex/token endpoint
// with the session token as a Bearer header. The bearer() plugin on the server
// converts this to a signed session cookie, and the convex plugin issues a JWT
// that can be verified via /api/auth/convex/jwks (same as the dashboard auth).
func exchangeSessionForJWT(ctx context.Context, authURL, sessionToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", authURL+"/api/auth/convex/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("User-Agent", useragent.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result jwtTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty JWT token in response")
	}
	return result.Token, nil
}

func createPATFromJWT(ctx context.Context, cfg *config.Config, jwtToken string) (string, error) {
	apiURL := cfg.BaseURL() + "/v1"

	body, _ := json.Marshal(map[string]string{
		"name": "CLI (auto)",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/user/tokens", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", useragent.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 201 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result createdTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Data.Token, nil
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
