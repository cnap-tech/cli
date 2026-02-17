package useragent

import (
	"fmt"
	"os"
	"runtime"
)

var version = "dev"

// SetVersion sets the CLI version used in the User-Agent string.
// Called from root command with build-time injected version.
func SetVersion(v string) { version = v }

// String returns a structured User-Agent string for HTTP requests.
// Format: CNAP CLI/{version} ({os}; {arch}; {hostname})
// Example: CNAP CLI/1.2.0 (darwin; arm64; Robins-MacBook-Pro.local)
func String() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("CNAP CLI/%s (%s; %s; %s)", version, runtime.GOOS, runtime.GOARCH, host)
}
