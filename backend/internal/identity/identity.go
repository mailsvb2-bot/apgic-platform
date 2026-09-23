package identity

import (
	"errors"
	"sort"
)

type Role string

const (
	RoleClient     Role = "CLIENT"
	RoleSpecialist Role = "SPECIALIST"
	RoleAuthor     Role = "AUTHOR"
	RoleStudent    Role = "STUDENT"
)

var ErrInvalidIdentity = errors.New("identity id is required")
var ErrInvalidRole = errors.New("unknown identity role")

type Identity struct {
	ID      string
	version uint64
	roles   map[Role]struct{}
}

func New(id string, initial ...Role) (*Identity, error) {
	if id == "" {
		return nil, ErrInvalidIdentity
	}
	i := &Identity{ID: id, version: 1, roles: make(map[Role]struct{})}
	for _, role := range initial {
		if _, err := i.AddRole(role); err != nil {
			return nil, err
		}
	}
	return i, nil
}

func (i *Identity) AddRole(role Role) (bool, error) {
	if !validRole(role) {
		return false, ErrInvalidRole
	}
	if _, exists := i.roles[role]; exists {
		return false, nil
	}
	i.roles[role] = struct{}{}
	i.version++
	return true, nil
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
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func (i *Identity) Version() uint64 { return i.version }

func validRole(role Role) bool {
	switch role {
	case RoleClient, RoleSpecialist, RoleAuthor, RoleStudent:
		return true
	default:
		return false
	}
}
