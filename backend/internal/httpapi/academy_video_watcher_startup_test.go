package httpapi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// A committed denial must be observed before the first periodic poll. This
// regression test deliberately uses a long interval to expose startup races;
// it is not a substitute for a PostgreSQL Entitlement/HTTP integration test.
func TestAcademyVideoWatcherDeniesBeforeFirstTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var checks atomic.Int32
	done := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, time.Hour, func() bool {
		    checks.Add(1)
		    return false
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked stream waited for poll tick")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("revoked stream context remains active: %v", ctx.Err())
	}
	if checks.Load() != 1 {
		t.Fatalf("startup authorization checked %d times, want 1", checks.Load())
	}
}

// An unexpected canonical-store failure must also deny an already-open stream.
func TestAcademyVideoWatcherFailsClosedOnRecheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var available atomic.Bool
	available.Store(true)
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, time.Millisecond, func() bool {
		    select {
		    case <-ready:
		        return available.Load()
		    default:
		        close(ready)
		        return true
		    }
		})
		close(done)
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("initial authorization never ran")
	}
	available.Store(false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canonical authorization failure did not cancel stream")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("authorization failure left stream active: %v", ctx.Err())
	}
}

// Cancellation before watcher startup must not read the canonical store or
// invoke a second cancellation. This matters when the HTTP client disconnects
// while the handler is preparing its streaming goroutine.
func TestAcademyVideoWatcherAlreadyCanceledDoesNotReauthorize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var checks atomic.Int32
	done := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, time.Hour, func() bool {
			checks.Add(1)
			return true
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled watcher did not exit promptly")
	}
	if got := checks.Load(); got != 0 {
		t.Fatalf("canceled watcher queried canonical store %d times, want 0", got)
	}
}
