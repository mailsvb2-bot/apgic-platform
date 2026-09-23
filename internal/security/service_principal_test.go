package security

import "testing"

func TestServicePrincipalRejectsWildcardScope(t *testing.T) {
	if _, err := NewServicePrincipal("connector-1", []string{"*"}); err == nil {
		t.Fatal("wildcard service scope must be rejected")
	}
	principal, err := NewServicePrincipal("connector-1", []string{"booking:read", "booking:write"})
	if err != nil {
		t.Fatal(err)
	}
	if !principal.HasScope("booking:read") {
		t.Fatal("expected scoped permission")
	}
}
