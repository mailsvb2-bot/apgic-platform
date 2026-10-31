package main

import (
	"testing"
)

func TestDurableProviderOutboxHasOnlyOneProcessOwner(t *testing.T) {
	dir := t.TempDir()
	releaseOne, err := lockProviderState(dir)
	if err != nil {
		t.Fatalf("first provider failed to acquire exclusive sequence ownership: %v", err)
	}
	if releaseOther, err := lockProviderState(dir); err == nil {
		releaseOther()
		releaseOne()
		t.Fatal("second provider accepted the same outbox without shared transaction authority")
	}
	releaseOne()
	releaseRestarted, err := lockProviderState(dir)
	if err != nil {
		t.Fatalf("provider could not resume after previous process stops: %v", err)
	}
	releaseRestarted()
}
