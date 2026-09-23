package identity

import "testing"

func TestOneIdentityCanAccumulateRolesIdempotently(t *testing.T) {
	id, err := New("person-1", RoleClient)
	if err != nil {
		t.Fatal(err)
	}
	if added, err := id.AddRole(RoleSpecialist); err != nil || !added {
		t.Fatalf("add specialist role: added=%v err=%v", added, err)
	}
	if added, err := id.AddRole(RoleSpecialist); err != nil || added {
		t.Fatalf("duplicate role must be idempotent: added=%v err=%v", added, err)
	}
	if !id.HasRole(RoleClient) || !id.HasRole(RoleSpecialist) {
		t.Fatal("roles must stay attached to the same identity")
	}
}
