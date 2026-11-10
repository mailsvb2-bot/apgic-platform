package httpapi

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A revocation between the request's initial authorization and the watcher
// starting must not wait for its first periodic poll.
func TestAcademyVideoWatcherRejectsAlreadyRevokedAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var checks atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		academyWatchVideoEntitlement(ctx, cancel, time.Hour, func() bool {
			checks.Add(1)
			return false
		})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked video watcher waited for first poll instead of cancelling")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("revoked watcher did not cancel stream: %v", ctx.Err())
	}
	if checks.Load() != 1 {
		t.Fatalf("expected one immediate canonical authorization check, got %d", checks.Load())
	}
}

func TestAcademyVideoWatcherDoesNotCheckCancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var checks atomic.Int32
	academyWatchVideoEntitlement(ctx, cancel, time.Hour, func() bool {
		checks.Add(1)
		return true
	})
	if checks.Load() != 0 {
		t.Fatalf("cancelled request made %d unnecessary authorization checks", checks.Load())
	}
}
