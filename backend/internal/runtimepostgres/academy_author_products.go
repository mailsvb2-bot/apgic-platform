package runtimepostgres

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Keep course creation and author discovery on precisely the same canonical
// Commerce ownership predicate. Neither route grants or changes ownership.
const academyPublishedProductEligibility = `p.status='PUBLISHED'
 AND ((p.owner_type='IDENTITY' AND p.owner_id=$1::uuid)
 OR (p.owner_type='ORGANIZATION' AND EXISTS (
   SELECT 1 FROM organization_ownerships o
   JOIN organization_memberships m
     ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id
   WHERE o.organization_id=p.owner_id AND o.identity_id=$1::uuid
     AND o.status='ACTIVE' AND m.status='ACTIVE'
 )))`

// AcademyAuthorProducts lists only products for which AcademyCreateCourse
// would accept the same signed identity. It never returns foreign product data.
func (c *Checker) AcademyAuthorProducts(actor string) ([]academy.AuthorProduct, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	if actor == "" {
		return nil, academy.ErrDenied
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT p.id::text,p.name
 FROM products p WHERE `+academyPublishedProductEligibility+`
 ORDER BY p.published_at DESC,p.id LIMIT 100`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products := make([]academy.AuthorProduct, 0)
	for rows.Next() {
		var p academy.AuthorProduct
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}
