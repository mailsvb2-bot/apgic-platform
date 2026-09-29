package organization

import (
	"errors"
	"strings"
)

type DirectionStatus string

const (
	DirectionActive   DirectionStatus = "ACTIVE"
	DirectionArchived DirectionStatus = "ARCHIVED"
)

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrDirectionNotFound    = errors.New("organization direction not found")
	ErrHardDeleteForbidden  = errors.New("hard delete forbidden: archive direction to preserve business truth")
	ErrInvalidOrganization  = errors.New("invalid organization")
	ErrInvalidDirection     = errors.New("invalid organization direction")
	ErrOwnerRequired        = errors.New("active organization owner required")
)

type Organization struct {
	ID         string
	Name       string
	Directions map[string]*Direction
}

type Direction struct {
	ID                string
	Name              string
	Type              string
	Status            DirectionStatus
	HasDependentTruth bool
}

type Snapshot struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Status     string              `json:"status"`
	Directions []DirectionSnapshot `json:"directions"`
}

type DirectionSnapshot struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Type           string `json:"direction_type"`
	Status         string `json:"status"`
}

func New(id, name string) *Organization {
	return &Organization{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name), Directions: make(map[string]*Direction)}
}

func NormalizeOrganizationName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidOrganization
	}
	return name, nil
}

func NormalizeDirection(name, directionType string) (string, string, error) {
	name = strings.TrimSpace(name)
	directionType = strings.ToUpper(strings.TrimSpace(directionType))
	if name == "" || directionType == "" {
		return "", "", ErrInvalidDirection
	}
	return name, directionType, nil
}

func (o *Organization) AddDirection(id, name string) {
	o.AddTypedDirection(id, name, "GENERAL")
}

func (o *Organization) AddTypedDirection(id, name, directionType string) {
	if _, exists := o.Directions[id]; exists {
		return
	}
	o.Directions[id] = &Direction{ID: id, Name: name, Type: directionType, Status: DirectionActive}
}

func (o *Organization) ArchiveDirection(id string) error {
	direction, ok := o.Directions[id]
	if !ok {
		return ErrDirectionNotFound
	}
	direction.Status = DirectionArchived
	return nil
}

func (o *Organization) HardDeleteDirection(id string) error {
	if _, ok := o.Directions[id]; !ok {
		return ErrDirectionNotFound
	}
	return ErrHardDeleteForbidden
}
