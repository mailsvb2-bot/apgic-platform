package connector

import "testing"

func TestConnectorInstanceUsesRegisteredCapabilityAndMinimalExecuteScope(t *testing.T) {
	instance, err := NewInstance(
		"connector-1",
		CapabilityNotification,
		"provider-a",
		"config/provider-a",
	)
	if err != nil {
		t.Fatal(err)
	}
	if instance.ExecuteScope != "connector:execute:NOTIFICATION_PROVIDER" {
		t.Fatalf("unexpected execute scope: %q", instance.ExecuteScope)
	}
	if instance.Status != StatusConfiguring {
		t.Fatalf("unexpected initial status: %q", instance.Status)
	}

	if _, err := NewInstance(
		"connector-2",
		CapabilityClass("UNREGISTERED_PROVIDER"),
		"provider-a",
		"config/provider-a",
	); err != ErrInvalidConnector {
		t.Fatalf("unregistered capability must fail, got %v", err)
	}
}

func TestConnectorInstanceRequiresConfigReference(t *testing.T) {
	if _, err := NewInstance(
		"connector-1",
		CapabilityNotification,
		"provider-a",
		"",
	); err != ErrInvalidConnector {
		t.Fatalf("missing config ref must fail, got %v", err)
	}
}
