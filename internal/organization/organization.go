package organization

import (
	"errors"
	"strings"
)

var (
	ErrInvalidOrganization = errors.New("invalid organization")
	ErrDirectionNotFound    = errors.New("direction not found")
)

type DirectionState string

const (
	DirectionActive   DirectionState = "ACTIVE"
	DirectionArchived DirectionState = "ARCHIVED"
)

type Direction struct {
	ID    string
	Kind  string
	State DirectionState
}

type Organization struct {
	ID         string
	directions map[string]Direction
}

func New(id string) (*Organization, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidOrganization
	}
	return &Organization{ID: id, directions: make(map[string]Direction)}, nil
}

func (o *Organization) AddDirection(id, kind string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(kind) == "" {
		return ErrInvalidOrganization
	}
	o.directions[id] = Direction{ID: id, Kind: kind, State: DirectionActive}
	return nil
}

func (o *Organization) ArchiveDirection(id string) error {
	direction, ok := o.directions[id]
	if !ok {
		return ErrDirectionNotFound
	}
	direction.State = DirectionArchived
	o.directions[id] = direction
	return nil
}

func (o *Organization) Direction(id string) (Direction, bool) {
	direction, ok := o.directions[id]
	return direction, ok
}
