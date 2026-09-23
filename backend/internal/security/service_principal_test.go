package security

import "testing"

func TestServicePrincipalRejectsWildcardAndEmptyScopes(t *testing.T) {
	if _,err:=NewServicePrincipal("connector-1",[]string{"*"}); err==nil { t.Fatal("wildcard scope must fail") }
	if _,err:=NewServicePrincipal("connector-1",[]string{}); err==nil { t.Fatal("empty scopes must fail") }
	p,err:=NewServicePrincipal("connector-1",[]string{"booking:read","booking:write"})
	if err!=nil { t.Fatal(err) }
	if !p.HasScope("booking:read") { t.Fatal("expected scoped permission") }
}
