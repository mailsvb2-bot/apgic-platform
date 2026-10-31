package runtimepostgres

import (
	"database/sql"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

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
	rows, err := tx.QueryContext(ctx, `SELECT c.id::text, c.title,
	 COUNT(DISTINCT e.id),
	 COUNT(DISTINCT e.id) FILTER (WHERE e.state='COMPLETED'),
	 COUNT(DISTINCT e.id) FILTER (WHERE academy_entitlement_is_active(e.entitlement_id)),
	 COUNT(DISTINCT a.id) FILTER (WHERE a.result='PENDING'),
	 COUNT(DISTINCT a.id) FILTER (WHERE a.result='PASSED')
	 FROM academy_courses c
	 LEFT JOIN academy_course_versions v ON v.course_id=c.id
	 LEFT JOIN academy_enrollments e ON e.version_id=v.id
	 LEFT JOIN academy_attempts a ON a.enrollment_id=e.id
	 WHERE c.author_id=$1::uuid
	 GROUP BY c.id,c.title,c.created_at
	 ORDER BY c.created_at DESC LIMIT 100`, author)
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
	rows, err = tx.QueryContext(ctx, `SELECT c.id::text, o.currency,
	 COUNT(sv.id), COALESCE(SUM(o.amount_minor),0)
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
	 GROUP BY c.id,o.currency ORDER BY c.id,o.currency`, author)
	if err != nil {
		return academy.AuthorReport{}, err
	}
	for rows.Next() {
		var item academy.CapturedGrossEvidence
		if err = rows.Scan(&item.CourseID, &item.Currency, &item.CapturedCount, &item.CapturedGrossMinor); err != nil {
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
