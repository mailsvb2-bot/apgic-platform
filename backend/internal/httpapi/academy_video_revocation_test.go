package httpapi

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcademyWatchVideoEntitlementCancelsStalledStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var authorized atomic.Bool
	authorized.Store(true)
	done := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, 5*time.Millisecond, authorized.Load)
		close(done)
	}()
	authorized.Store(false)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("revoked stream did not cancel")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("revocation watcher leaked")
	}
}
