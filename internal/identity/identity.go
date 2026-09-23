package identity

import (
	"errors"
	"strings"
)

type Role string

const (
	RoleClient             Role = "CLIENT"
	RoleSpecialist         Role = "SPECIALIST"
	RoleAuthor             Role = "AUTHOR"
	RoleStudent            Role = "STUDENT"
	RoleOrganizationMember Role = "ORGANIZATION_MEMBER"
)

var ErrInvalidIdentity = errors.New("invalid identity")

type Identity struct {
	ID    string
	roles map[Role]struct{}
}

func New(id string) (*Identity, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidIdentity
	}
	return &Identity{ID: id, roles: make(map[Role]struct{})}, nil
}

func (i *Identity) GrantRole(role Role) error {
	if role == "" {
		return ErrInvalidIdentity
	}
	i.roles[role] = struct{}{}
	return nil
}

func (i *Identity) HasRole(role Role) bool {
	_, ok := i.roles[role]
	return ok
}

func (i *Identity) Roles() []Role {
	out := make([]Role, 0, len(i.roles))
	for role := range i.roles {
		out = append(out, role)
	}
	return out
}
