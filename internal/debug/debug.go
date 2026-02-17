// Package debug configures structured logging for the CLI.
//
// When --debug or CNAP_DEBUG=1 is set, debug-level logs are emitted to stderr.
// Otherwise, only warn and above are shown (effectively silent).
package debug

import (
	"io"
	"log/slog"
	"os"
)

// Enabled reports whether debug mode is active.
var Enabled bool

// Init configures the global slog logger.
// Call once from the root command's PersistentPreRun.
func Init(flagEnabled bool) {
	Enabled = flagEnabled || os.Getenv("CNAP_DEBUG") != ""

	w := io.Discard
	level := slog.LevelWarn
	if Enabled {
		w = os.Stderr
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})))
}
