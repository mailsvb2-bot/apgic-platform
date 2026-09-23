package commerce

import (
	"errors"
	"strings"
)

type OwnerType string

const (
	OwnerIdentity     OwnerType = "IDENTITY"
	OwnerOrganization OwnerType = "ORGANIZATION"
)

var ErrOwnershipIncomplete = errors.New("product ownership roles are incomplete")

type Ownership struct {
	OwnerType             OwnerType
	OwnerID               string
	CommercialOwnerRef    string
	AuthorRefs            []string
	RevenueBeneficiaryRef string
}

func (o Ownership) Validate() error {
	if (o.OwnerType != OwnerIdentity && o.OwnerType != OwnerOrganization) ||
		strings.TrimSpace(o.OwnerID) == "" ||
		strings.TrimSpace(o.CommercialOwnerRef) == "" ||
		len(o.AuthorRefs) == 0 ||
		strings.TrimSpace(o.RevenueBeneficiaryRef) == "" {
		return ErrOwnershipIncomplete
	}
	for _, author := range o.AuthorRefs {
		if strings.TrimSpace(author) == "" {
			return ErrOwnershipIncomplete
		}
	}
	return nil
}
