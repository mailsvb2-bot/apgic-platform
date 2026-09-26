package runtimepostgres

import "testing"

func TestRequiresDatabaseOnlyForRuntimeEnvironments(t *testing.T) {
	for _, value := range []string{"STAGING", "staging", "PRODUCTION", " production "} {
		if !RequiresDatabase(value) {
			t.Fatalf("%q must require database", value)
		}
	}
	for _, value := range []string{"", "CI", "DEV", "LOCAL"} {
		if RequiresDatabase(value) {
			t.Fatalf("%q must not require database", value)
		}
	}
}

func TestRequiredCanonicalTablesAreStable(t *testing.T) {
	want := map[string]bool{
		"identities": true, "outbox_events": true, "audit_records": true,
		"ledger_entries": true, "booking_slots": true, "booking_holds": true,
		"bookings": true,
	}
	if len(requiredTables) != len(want) {
		t.Fatalf("required tables = %v", requiredTables)
	}
	for _, table := range requiredTables {
		if !want[table] {
			t.Fatalf("unexpected required table %q", table)
		}
	}
}
