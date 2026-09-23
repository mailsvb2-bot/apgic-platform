package security

import (
	"errors"
	"strings"
)

var ErrInvalidServicePrincipal = errors.New("invalid service principal")

type ServicePrincipal struct {
	ID     string
	Scopes map[string]struct{}
}

func NewServicePrincipal(id string, scopes []string) (ServicePrincipal, error) {
	if strings.TrimSpace(id) == "" || len(scopes) == 0 {
		return ServicePrincipal{}, ErrInvalidServicePrincipal
	}
	principal := ServicePrincipal{ID: id, Scopes: make(map[string]struct{}, len(scopes))}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || scope == "*" {
			return ServicePrincipal{}, ErrInvalidServicePrincipal
		}
		principal.Scopes[scope] = struct{}{}
	}
	return principal, nil
}

func (p ServicePrincipal) HasScope(scope string) bool {
	_, ok := p.Scopes[scope]
	return ok
}
