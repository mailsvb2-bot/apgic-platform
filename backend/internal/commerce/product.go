package commerce

import "errors"

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
		o.OwnerID == "" || o.CommercialOwnerRef == "" || o.RevenueBeneficiaryRef == "" {
		return ErrOwnershipIncomplete
	}
	return nil
}
