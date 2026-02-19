//go:build !windows

package installs

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/coder/websocket"
)

// monitorResize listens for SIGWINCH signals and sends resize events over the WebSocket.
func monitorResize(ctx context.Context, conn *websocket.Conn, stop <-chan struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-sigCh:
			sendResize(ctx, conn)
		case <-stop:
			return
		}
	}
}
