package identity

import "testing"

func TestSingleIdentityCanHoldMultipleRoles(t *testing.T) {
	id, err := New("person-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []Role{RoleClient, RoleSpecialist, RoleAuthor} {
		if err := id.GrantRole(role); err != nil {
			t.Fatal(err)
		}
	}
	if id.ID != "person-1" {
		t.Fatalf("identity changed: %q", id.ID)
	}
	if !id.HasRole(RoleClient) || !id.HasRole(RoleSpecialist) || !id.HasRole(RoleAuthor) {
		t.Fatal("expected all roles to remain attached to the same identity")
	}
}
