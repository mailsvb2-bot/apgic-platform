package runtimepostgres

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyVerifyYandexMediaBatchBoundedAndDeduplicated(t *testing.T) {
	items := make([]academy.CloudMedia, 0, 41)
	for i := 0; i < 40; i++ {
		items = append(items, academy.CloudMedia{Provider: "YANDEX_DISK", ProviderFileRef: fmt.Sprintf("disk:/videos/%03d.mp4", i)})
	}
	items = append(items, items[0])
	var active, peak, verified atomic.Int32
	err := academyVerifyYandexMediaBatch(context.Background(), items, func(ctx context.Context, path string) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		verified.Add(1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Millisecond):
			return nil
		}
	})
	if err != nil || verified.Load() != 40 || peak.Load() > 4 || peak.Load() < 2 {
		t.Fatalf("batch verification: err=%v verified=%d peak=%d", err, verified.Load(), peak.Load())
	}
}

func TestAcademyVerifyYandexMediaBatchValidatesBeforeProviderCall(t *testing.T) {
	var called atomic.Int32
	items := []academy.CloudMedia{
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/ok.mp4"},
		{Provider: "EXTERNAL", ProviderFileRef: "disk:/invalid.mp4"},
	}
	err := academyVerifyYandexMediaBatch(context.Background(), items, func(context.Context, string) error {
		called.Add(1)
		return nil
	})
	if !errors.Is(err, academy.ErrInvalid) || called.Load() != 0 {
		t.Fatalf("invalid batch reached provider: err=%v calls=%d", err, called.Load())
	}
}

func TestAcademyVerifyYandexMediaBatchFailsClosed(t *testing.T) {
	items := []academy.CloudMedia{
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/first.mp4"},
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/bad.mp4"},
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/third.mp4"},
	}
	err := academyVerifyYandexMediaBatch(context.Background(), items, func(_ context.Context, path string) error {
		if path == "disk:/bad.mp4" {
			return errors.New("upstream error: must not escape")
		}
		return nil
	})
	if !errors.Is(err, academy.ErrDenied) || err.Error() == "upstream error: must not escape" {
		t.Fatalf("provider error leaked or accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = academyVerifyYandexMediaBatch(ctx, items, func(context.Context, string) error {
		t.Error("cancelled request reached provider")
		return nil
	})
	if !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("cancelled import accepted: %v", err)
	}
}
