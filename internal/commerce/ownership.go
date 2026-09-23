package commerce

import (
	"errors"
	"strings"
)

var ErrOwnershipIncomplete = errors.New("product ownership is incomplete")

type Ownership struct {
	OwnerID              string
	CommercialOwnerID    string
	RevenueBeneficiaryID string
	AuthorID             *string
}

func (o Ownership) Validate() error {
	if strings.TrimSpace(o.OwnerID) == "" ||
		strings.TrimSpace(o.CommercialOwnerID) == "" ||
		strings.TrimSpace(o.RevenueBeneficiaryID) == "" {
		return ErrOwnershipIncomplete
	}
	return nil
}
