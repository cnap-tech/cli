package debug

import (
	"log/slog"
	"net/http"
	"time"
)

// Transport wraps an http.RoundTripper and logs request/response details
// when debug mode is enabled.
type Transport struct {
	Inner http.RoundTripper
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !Enabled {
		return t.inner().RoundTrip(req)
	}

	slog.Debug("HTTP request",
		"method", req.Method,
		"url", req.URL.String(),
	)

	start := time.Now()
	resp, err := t.inner().RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		slog.Debug("HTTP error", "method", req.Method, "url", req.URL.String(), "error", err, "duration", elapsed)
		return nil, err
	}

	slog.Debug("HTTP response",
		"method", req.Method,
		"url", req.URL.String(),
		"status", resp.StatusCode,
		"duration", elapsed,
	)

	return resp, nil
}

func (t *Transport) inner() http.RoundTripper {
	if t.Inner != nil {
		return t.Inner
	}
	return http.DefaultTransport
}

// Install replaces http.DefaultClient's transport with a debug-logging wrapper.
// This covers manual http.DefaultClient.Do() calls (e.g. device flow).
func Install() {
	http.DefaultClient.Transport = &Transport{Inner: http.DefaultClient.Transport}
}

// Client returns an *http.Client with the debug transport.
// Use this when a library creates its own http.Client (e.g. oapi-codegen).
func Client() *http.Client {
	if !Enabled {
		return http.DefaultClient
	}
	return &http.Client{Transport: &Transport{Inner: http.DefaultTransport}}
}
