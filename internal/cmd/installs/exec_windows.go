package installs

import (
	"context"
	"os"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// monitorResize polls terminal size every 250ms and sends resize events when dimensions change.
// Windows has no SIGWINCH equivalent, so polling is the standard approach (used by kubectl).
func monitorResize(ctx context.Context, conn *websocket.Conn, stop <-chan struct{}) {
	fd := int(os.Stdout.Fd())
	w, h, err := term.GetSize(fd)
	if err != nil {
		return
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			newW, newH, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			if newW != w || newH != h {
				w, h = newW, newH
				sendResize(ctx, conn)
			}
		case <-stop:
			return
		}
	}
}
