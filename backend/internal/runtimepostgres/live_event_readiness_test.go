package runtimepostgres

import (
	"context"
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

func TestR9ReadinessFailsClosedWithoutCanonicalDatabase(t *testing.T) {
	var uninitialized *Checker
	if !errors.Is(uninitialized.LiveEventsReady(context.Background()), liveevents.ErrUnavailable) {
		t.Fatal("nil checker must not advertise R9 availability")
	}
	checker := &Checker{}
	if !errors.Is(checker.LiveEventsReady(context.Background()), liveevents.ErrUnavailable) {
		t.Fatal("nil database must not advertise R9 availability")
	}
}
