package runtimepostgres

import (
	"context"
	"os"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestConnectorPersistenceKeepsProviderKindBehindCanonicalCapability(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CONNECTOR_DELIVERY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("connector PostgreSQL integration database not configured")
	}

	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
	})

	firstID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []struct {
		id           string
		providerKind string
		configRef    string
	}{
		{firstID, "vendor-alpha", "secretref://connector/" + firstID},
		{secondID, "vendor-beta", "secretref://connector/" + secondID},
	} {
		if _, err := store.db.Exec(
			`INSERT INTO connector_instances (
				id, capability_class, provider_kind, status, config_ref
			) VALUES ($1::uuid, 'COMMUNICATION_PROVIDER', $2, 'ACTIVE', $3)`,
			row.id,
			row.providerKind,
			row.configRef,
		); err != nil {
			t.Fatalf("persist %s communication provider: %v", row.providerKind, err)
		}
	}

	rows, err := store.db.Query(
		`SELECT provider_kind, capability_class, execute_scope
		   FROM connector_instances
		  WHERE id IN ($1::uuid, $2::uuid)
		  ORDER BY provider_kind`,
		firstID,
		secondID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var providerKind, capabilityClass, executeScope string
		if err := rows.Scan(&providerKind, &capabilityClass, &executeScope); err != nil {
			t.Fatal(err)
		}
		if providerKind != "vendor-alpha" && providerKind != "vendor-beta" {
			t.Fatalf("unexpected provider kind %q", providerKind)
		}
		if capabilityClass != "COMMUNICATION_PROVIDER" {
			t.Fatalf("provider %q canonical capability=%q want COMMUNICATION_PROVIDER", providerKind, capabilityClass)
		}
		if executeScope != "connector:execute:COMMUNICATION_PROVIDER" {
			t.Fatalf("provider %q execute scope=%q", providerKind, executeScope)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Fatalf("provider-neutral connector rows=%d want=2", seen)
	}

	brandCapabilityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO connector_instances (
			id, capability_class, provider_kind, status, config_ref
		) VALUES ($1::uuid, 'ClientPlatform', 'vendor-gamma', 'ACTIVE', $2)`,
		brandCapabilityID,
		"secretref://connector/"+brandCapabilityID,
	); err == nil {
		t.Fatal("brand-coupled capability class unexpectedly persisted")
	}
}
