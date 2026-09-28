package runtimepostgres

import (
	"context"
	"os"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestIdentityRoleGrantKeepsSingleAccountTruth(t *testing.T) {
	databaseURL := os.Getenv("APGIC_IDENTITY_ROLE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("identity role integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	identityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO identities (id) VALUES ($1::uuid)`,
		identityID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO identity_roles (identity_id, role_code) VALUES ($1::uuid, 'CLIENT')`,
		identityID,
	); err != nil {
		t.Fatal(err)
	}

	for _, grant := range []struct {
		role        identity.Role
		wantVersion uint64
	}{
		{role: identity.RoleSpecialist, wantVersion: 2},
		{role: identity.RoleAuthor, wantVersion: 3},
		{role: identity.RoleStudent, wantVersion: 4},
	} {
		created, version, err := store.GrantIdentityRole(identityID, grant.role)
		if err != nil {
			t.Fatal(err)
		}
		if !created || version != grant.wantVersion {
			t.Fatalf("grant %s created=%v version=%d want=%d", grant.role, created, version, grant.wantVersion)
		}
	}

	created, version, err := store.GrantIdentityRole(identityID, identity.RoleSpecialist)
	if err != nil {
		t.Fatal(err)
	}
	if created || version != 4 {
		t.Fatalf("idempotent specialist replay created=%v version=%d", created, version)
	}

	roles, version, err := store.IdentityRoles(identityID)
	if err != nil {
		t.Fatal(err)
	}
	wantRoles := []identity.Role{
		identity.RoleAuthor,
		identity.RoleClient,
		identity.RoleSpecialist,
		identity.RoleStudent,
	}
	if len(roles) != len(wantRoles) {
		t.Fatalf("roles=%v", roles)
	}
	for i := range wantRoles {
		if roles[i] != wantRoles[i] {
			t.Fatalf("roles=%v want=%v", roles, wantRoles)
		}
	}
	if version != 4 {
		t.Fatalf("identity version=%d want=4", version)
	}

	var identityCount int
	if err := store.db.QueryRow(
		`SELECT count(*) FROM identities WHERE id = $1::uuid`,
		identityID,
	).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if identityCount != 1 {
		t.Fatalf("canonical identity rows=%d want=1", identityCount)
	}
}

func TestIdentityRoleGrantRejectsUnknownAccountAndRole(t *testing.T) {
	databaseURL := os.Getenv("APGIC_IDENTITY_ROLE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("identity role integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	missingID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GrantIdentityRole(missingID, identity.RoleSpecialist); err != ErrIdentityNotFound {
		t.Fatalf("missing identity err=%v", err)
	}
	if _, _, err := store.GrantIdentityRole(missingID, identity.Role("ADMIN")); err != identity.ErrInvalidRole {
		t.Fatalf("unknown role err=%v", err)
	}
}
