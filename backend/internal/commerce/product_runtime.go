package commerce

import (
	"errors"
	"strings"
	"time"
)

type ProductStatus string

const (
	ProductDraft     ProductStatus = "DRAFT"
	ProductPublished ProductStatus = "PUBLISHED"
)

var (
	ErrProductInvalid            = errors.New("invalid product")
	ErrProductNotFound           = errors.New("product not found")
	ErrProductOwnerRequired      = errors.New("active organization owner required")
	ErrProductDirectionInvalid   = errors.New("active organization direction required")
	ErrProductAlreadyPublished   = errors.New("product already published")
)

type ProductSnapshot struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	Status                ProductStatus `json:"status"`
	OwnerType             OwnerType     `json:"owner_type"`
	OwnerID               string        `json:"owner_id"`
	CommercialOwnerRef    string        `json:"commercial_owner_ref"`
	AuthorRefs            []string      `json:"author_refs"`
	RevenueBeneficiaryRef string        `json:"revenue_beneficiary_ref"`
	OrganizationDirectionID string      `json:"organization_direction_id"`
	PublishedAt           *time.Time    `json:"published_at,omitempty"`
}

type OrganizationProductDraft struct {
	Name                  string
	DirectionID           string
	CommercialOwnerRef    string
	AuthorRefs            []string
	RevenueBeneficiaryRef string
}

func NormalizeOrganizationProductDraft(input OrganizationProductDraft) (OrganizationProductDraft, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.DirectionID = strings.TrimSpace(input.DirectionID)
	input.CommercialOwnerRef = strings.TrimSpace(input.CommercialOwnerRef)
	input.RevenueBeneficiaryRef = strings.TrimSpace(input.RevenueBeneficiaryRef)
	authors := make([]string, 0, len(input.AuthorRefs))
	seen := map[string]struct{}{}
	for _, author := range input.AuthorRefs {
		author = strings.TrimSpace(author)
		if author == "" {
			return OrganizationProductDraft{}, ErrProductInvalid
		}
		if _, ok := seen[author]; ok {
			return OrganizationProductDraft{}, ErrProductInvalid
		}
		seen[author] = struct{}{}
		authors = append(authors, author)
	}
	input.AuthorRefs = authors
	if input.Name == "" || input.DirectionID == "" || input.CommercialOwnerRef == "" ||
		input.RevenueBeneficiaryRef == "" || len(input.AuthorRefs) == 0 {
		return OrganizationProductDraft{}, ErrProductInvalid
	}
	if err := (Ownership{
		OwnerType:             OwnerOrganization,
		OwnerID:               "organization-bound-at-runtime",
		CommercialOwnerRef:    input.CommercialOwnerRef,
		AuthorRefs:            input.AuthorRefs,
		RevenueBeneficiaryRef: input.RevenueBeneficiaryRef,
	}).Validate(); err != nil {
		return OrganizationProductDraft{}, ErrProductInvalid
	}
	return input, nil
}
