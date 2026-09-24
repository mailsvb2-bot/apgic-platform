package calendar

import (
	"testing"
	"time"
)

func TestCalendarFailureKeepsSyncRetryable(t *testing.T) {
	t0 := time.Now().UTC()
	job, err := NewJob(Job{
		ID: "sync-1", BookingID: "booking-1", BookingVersion: 7,
		ProviderInstanceID: "calendar-a", TargetRef: "calendar/user-1",
		IdempotencyKey: "booking-1:v7", CreatedAt: t0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := job.ApplyProviderResult(StateFailedRetryable, "", t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.State != StateFailedRetryable || job.BookingVersion != 7 {
		t.Fatalf("calendar outage must not alter booking truth: %#v", job)
	}
}

func TestCalendarSyncSuccessRequiresExternalEventReference(t *testing.T) {
	t0 := time.Now().UTC()
	job, err := NewJob(Job{
		ID: "sync-1", BookingID: "booking-1", BookingVersion: 1,
		ProviderInstanceID: "calendar-a", TargetRef: "calendar/user-1",
		IdempotencyKey: "booking-1:v1", CreatedAt: t0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := job.ApplyProviderResult(StateSynced, "", t0.Add(time.Second)); err == nil {
		t.Fatal("SYNCED without provider event evidence must fail")
	}
}
