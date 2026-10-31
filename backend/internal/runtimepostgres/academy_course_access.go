package runtimepostgres

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Keep enrollment and the learner's purchase readback on one verified
// Commerce/Entitlement predicate. This projection never creates a payment,
// entitlement or enrollment and does not grant access by itself.
const academyVerifiedEnrollmentFrom = ` FROM academy_courses c
 JOIN academy_course_versions v ON v.course_id=c.id AND v.state='PUBLISHED'
 JOIN store_entitlements t ON t.product_ref=c.product_id::text
 JOIN store_transaction_verifications sv ON sv.id=t.verification_id
  AND sv.identity_id=t.identity_id AND sv.product_ref=t.product_ref
 JOIN orders order_row ON order_row.id=sv.order_id AND order_row.product_id=c.product_id
 JOIN payment_effects paid ON paid.id=sv.payment_effect_id AND paid.effect_kind='CAPTURED'
 WHERE c.state='PUBLISHED' AND t.identity_id=$1::uuid
 AND academy_entitlement_is_active(t.id)`

// AcademyMyCourseAccess returns only published courses backed by the same
// currently effective verified purchase required by AcademyEnroll. A refunded
// or otherwise revoked entitlement must disappear immediately.
func (c *Checker) AcademyMyCourseAccess(actor string) ([]string, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	if actor == "" {
		return nil, academy.ErrDenied
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT DISTINCT c.id::text `+academyVerifiedEnrollmentFrom+`
 ORDER BY c.id::text`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	courses := []string{}
	for rows.Next() {
		var courseID string
		if err := rows.Scan(&courseID); err != nil {
			return nil, err
		}
		courses = append(courses, courseID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return courses, nil
}
