package runtimepostgres

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Both the learner's catalog readback and the enrollment writer must select
// the same latest published version and effective canonical purchase.
// academy_entitlement_is_active is the Academy-wide captured Commerce evidence
// gate (including refund/revocation), defined by PostgreSQL migrations.
// A newer independent purchase must not silently reopen a terminal or
// refunded historical enrollment bound to the same course version.
const academyVerifiedEnrollmentFrom = ` FROM academy_courses c
 JOIN academy_course_versions v ON v.course_id=c.id AND v.state='PUBLISHED'
 JOIN store_entitlements t ON t.product_ref=c.product_id::text
 WHERE c.state='PUBLISHED' AND t.identity_id=$1::uuid
 AND academy_entitlement_is_active(t.id)
 AND v.version_no=(
   SELECT MAX(latest.version_no) FROM academy_course_versions latest
   WHERE latest.course_id=c.id AND latest.state='PUBLISHED'
 )
 AND NOT EXISTS (
   SELECT 1 FROM academy_enrollments existing
   WHERE existing.identity_id=t.identity_id AND existing.version_id=v.id
   AND (existing.state NOT IN ('ACTIVE','COMPLETED')
        OR NOT academy_entitlement_is_active(existing.entitlement_id))
 )`

// AcademyMyCourseAccess returns only published courses for which the exact
// AcademyEnroll action can succeed under the same purchase/version predicate.
// This projection cannot create a payment, entitlement or enrollment.
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
