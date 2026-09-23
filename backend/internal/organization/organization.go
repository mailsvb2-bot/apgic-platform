package organization

import "errors"

type DirectionStatus string

const (
	DirectionActive   DirectionStatus = "ACTIVE"
	DirectionArchived DirectionStatus = "ARCHIVED"
)

var (
	ErrDirectionNotFound  = errors.New("organization direction not found")
	ErrHardDeleteForbidden = errors.New("hard delete forbidden: archive direction to preserve business truth")
)

type Organization struct {
	ID         string
	Name       string
	Directions map[string]*Direction
}

type Direction struct {
	ID                string
	Name              string
	Status            DirectionStatus
	HasDependentTruth bool
}

func New(id, name string) *Organization {
	return &Organization{ID: id, Name: name, Directions: make(map[string]*Direction)}
}

func (o *Organization) AddDirection(id, name string) {
	if _, exists := o.Directions[id]; exists {
		return
	}
	o.Directions[id] = &Direction{ID: id, Name: name, Status: DirectionActive}
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
