package httpapi

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// The canonical stream watcher must cancel an already-open response when
// Entitlement is revoked, even if no additional provider bytes arrive.
// This tests the cancellation signal, not the live network write deadline.
func TestAcademyVideoEntitlementWatcherCancelsAfterRevocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var allowed atomic.Bool
	allowed.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		academyWatchVideoEntitlement(ctx, cancel, 5*time.Millisecond, allowed.Load)
	}()

	allowed.Store(false)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("watcher did not cancel the active stream after entitlement revocation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not exit after cancellation")
	}
}

// Revocation between the handler's initial check and watcher startup must
// fail closed without waiting for the first periodic recheck.
func TestAcademyVideoEntitlementWatcherRejectsInitiallyRevoked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		academyWatchVideoEntitlement(ctx, cancel, time.Hour, func() bool { return false })
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("initially revoked entitlement was not cancelled immediately")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not exit after initial revocation")
	}
}
