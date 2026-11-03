package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type academyIdleUntilCancelled struct {
	ctx context.Context
}

func (r academyIdleUntilCancelled) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func TestAcademyVideoIdleReaderCancelsStalledProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := academyVideoIdleReader{
		source:  academyIdleUntilCancelled{ctx: ctx},
		cancel:  cancel,
		timeout: 15 * time.Millisecond,
	}
	done := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 8))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stalled provider read must time out, got %v", err)
		}
		if ctx.Err() == nil {
			t.Fatal("stalled provider request was not cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("stalled provider pinned gateway beyond idle budget")
	}
}

func TestAcademyVideoIdleReaderResetsBudgetOnProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := academyVideoIdleReader{
		source:  bytes.NewBufferString("video-data"),
		cancel:  cancel,
		timeout: 100 * time.Millisecond,
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatal(err)
	}
	if output.String() != "video-data" {
		t.Fatalf("video bytes lost: %q", output.String())
	}
	time.Sleep(120 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("finished video read left a live cancellation timer")
	}
}

func TestAcademyVideoIdleReaderRejectsMissingDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := academyVideoIdleReader{source: bytes.NewBufferString("secret"), cancel: cancel}
	n, err := reader.Read(make([]byte, 16))
	if n != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unbounded idle read must fail closed: n=%d err=%v", n, err)
	}
	if ctx.Err() != nil {
		t.Fatal("invalid idle-reader configuration must not cancel unrelated context")
	}
}
