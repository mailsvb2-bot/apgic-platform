package runtimepostgres

import (
	"database/sql"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Keep the author's actionable metrics aligned with the same captured Commerce
// entitlement gate as learner access. Historical enrollments and graded work
// remain counted, but revoked enrollment cannot inflate active access or the
// instructor's pending grading queue.
const academyAuthorCourseMetricsQuery = `SELECT c.id::text, c.title,
 COUNT(DISTINCT e.id),
 COUNT(DISTINCT e.id) FILTER (WHERE e.state='COMPLETED'),
 COUNT(DISTINCT e.id) FILTER (
   WHERE e.state IN ('ACTIVE','COMPLETED')
     AND academy_entitlement_is_active(e.entitlement_id)),
 COUNT(DISTINCT a.id) FILTER (
   WHERE a.result='PENDING' AND e.state='ACTIVE'
     AND academy_entitlement_is_active(e.entitlement_id)),
 COUNT(DISTINCT a.id) FILTER (WHERE a.result='PASSED')
 FROM academy_courses c
 LEFT JOIN academy_course_versions v ON v.course_id=c.id
 LEFT JOIN academy_enrollments e ON e.version_id=v.id
 LEFT JOIN academy_attempts a ON a.enrollment_id=e.id
 WHERE c.author_id=$1::uuid
 GROUP BY c.id,c.title,c.created_at
 ORDER BY c.created_at DESC LIMIT 100`

// Captured payments and current enrolled access are distinct. A terminal
// Academy enrollment must not appear actionable merely because its original
// Commerce entitlement remains active. An unenrolled purchase stays in gross.
const academyAuthorCapturedGrossQuery = `SELECT c.id::text, o.currency,
	 COUNT(sv.id), COALESCE(SUM(o.amount_minor),0),
	 COUNT(sv.id) FILTER (WHERE academy_entitlement_is_active(ent.id)
	   AND EXISTS (
	     SELECT 1 FROM academy_enrollments e
	     JOIN academy_course_versions v ON v.id=e.version_id
	     WHERE v.course_id=c.id AND e.entitlement_id=ent.id
	       AND e.state IN ('ACTIVE','COMPLETED')
	   )),
	 COALESCE(SUM(o.amount_minor) FILTER (WHERE academy_entitlement_is_active(ent.id)
	   AND EXISTS (
	     SELECT 1 FROM academy_enrollments e
	     JOIN academy_course_versions v ON v.id=e.version_id
	     WHERE v.course_id=c.id AND e.entitlement_id=ent.id
	       AND e.state IN ('ACTIVE','COMPLETED')
	   )),0)
	 FROM academy_courses c
	 JOIN store_transaction_verifications sv ON sv.product_ref=c.product_id::text
	 JOIN store_entitlements ent ON ent.verification_id=sv.id
	   AND ent.identity_id=sv.identity_id AND ent.product_ref=sv.product_ref
	 JOIN orders o ON o.id=sv.order_id AND o.product_id=c.product_id
	 JOIN payment_attempts pa ON pa.id=sv.payment_attempt_id
	   AND pa.order_id=o.id AND pa.state='SUCCEEDED'
	   AND pa.amount_minor=o.amount_minor AND pa.currency=o.currency
	 JOIN payment_effects pe ON pe.id=sv.payment_effect_id
	   AND pe.attempt_id=pa.id AND pe.effect_kind='CAPTURED'
	   AND pe.ledger_entry_id=sv.ledger_entry_id
	 JOIN ledger_entries le ON le.id=sv.ledger_entry_id
	   AND le.provider_evidence_ref=pe.provider_evidence_ref
	 WHERE c.author_id=$1::uuid
	 GROUP BY c.id,o.currency ORDER BY c.id,o.currency`

// AcademyAuthorReport reads only the signed author's own courses. Financial
// amounts are verified gross captures for the exact order product, not income.
// A read-only repeatable-read transaction keeps metrics and captures coherent.
func (c *Checker) AcademyAuthorReport(author string) (academy.AuthorReport, error) {
	if c == nil || c.db == nil {
		return academy.AuthorReport{}, academy.ErrUnavailable
	}
	if author == "" {
		return academy.AuthorReport{}, academy.ErrDenied
	}
	ctx, cancel := academyContext()
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return academy.AuthorReport{}, err
	}
	defer tx.Rollback()

	report := academy.AuthorReport{
		Courses:       []academy.CourseReport{},
		GrossCaptures: []academy.CapturedGrossEvidence{},
		RevenueState:  academy.RevenueGrossOnly,
	}
	rows, err := tx.QueryContext(ctx, academyAuthorCourseMetricsQuery, author)
	if err != nil {
		return academy.AuthorReport{}, err
	}
	for rows.Next() {
		var item academy.CourseReport
		if err = rows.Scan(&item.CourseID, &item.Title, &item.Enrollments,
			&item.Completed, &item.ActiveAccess, &item.PendingAssessments,
			&item.PassedAssessments); err != nil {
			rows.Close()
			return academy.AuthorReport{}, err
		}
		report.Courses = append(report.Courses, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return academy.AuthorReport{}, err
	}
	rows.Close()

	// An entitlement product_ref alone is not proof of a course sale. Require
	// the immutable canonical order's product_id to match the course product.
	// Each store verification and entitlement is unique, so no JOIN multiplication.
	// Report the captured historical gross alongside its current entitlement
	// subset; never subtract this subset from gross and call it a refund or net
	// author income. Canonical Finance still owns all monetary reconciliation.
	rows, err = tx.QueryContext(ctx, academyAuthorCapturedGrossQuery, author)
	if err != nil {
		return academy.AuthorReport{}, err
	}
	for rows.Next() {
		var item academy.CapturedGrossEvidence
		if err = rows.Scan(&item.CourseID, &item.Currency, &item.CapturedCount, &item.CapturedGrossMinor,
			&item.ActiveCount, &item.ActiveGrossMinor); err != nil {
			rows.Close()
			return academy.AuthorReport{}, err
		}
		report.GrossCaptures = append(report.GrossCaptures, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return academy.AuthorReport{}, err
	}
	rows.Close()
	if err = tx.Commit(); err != nil {
		return academy.AuthorReport{}, err
	}
	return report, nil
}
