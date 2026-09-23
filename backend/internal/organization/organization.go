package organization

import "errors"

type DirectionStatus string

const (
	DirectionActive   DirectionStatus = "ACTIVE"
	DirectionArchived DirectionStatus = "ARCHIVED"
)

var (
	ErrDirectionNotFound = errors.New("organization direction not found")
	ErrDependentTruth    = errors.New("hard delete blocked: direction has dependent business truth")
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
	d, ok := o.Directions[id]
	if !ok {
		return ErrDirectionNotFound
	}
	d.Status = DirectionArchived
	return nil
}

func (o *Organization) HardDeleteDirection(id string) error {
	d, ok := o.Directions[id]
	if !ok {
		return ErrDirectionNotFound
	}
	if d.HasDependentTruth {
		return ErrDependentTruth
	}
	delete(o.Directions, id)
	return nil
}
