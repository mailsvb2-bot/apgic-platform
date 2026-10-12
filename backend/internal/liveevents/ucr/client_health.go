package ucr

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// TransportReady checks that the authenticated conference transport is
// reachable, not that a particular event's SFU media has been provisioned.
// Event-level capability and runtime proofs remain mandatory on every join.
func (c *Client) TransportReady(ctx context.Context) error {
	if c == nil || c.httpClient == nil || c.endpoint == nil {
		return liveevents.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	target := *c.endpoint
	target.Path = "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return liveevents.ErrUnavailable
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return liveevents.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return liveevents.ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32))
	if err != nil || strings.TrimSpace(string(body)) != "ok" {
		return liveevents.ErrUnavailable
	}
	return nil
}
