package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyLearnerCohorts exposes only cohorts for this learner's already
// purchased course version. No teacher roster or foreign learner IDs escape.
func (c *Checker) AcademyLearnerCohorts(actor, enrollmentID string) ([]academy.Cohort, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var allowed bool
	err := c.db.QueryRowContext(ctx, `SELECT academy_entitlement_is_active(e.entitlement_id)
 FROM academy_enrollments e WHERE e.id=$1::uuid AND e.identity_id=$2::uuid
 AND e.state IN ('ACTIVE','COMPLETED')`, enrollmentID, actor).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, academy.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, academy.ErrEntitlement
	}
	rows, err := c.db.QueryContext(ctx, `SELECT h.id::text,h.version_id::text,h.title,COALESCE(h.capacity,0),
 (SELECT count(*) FROM academy_cohort_memberships m WHERE m.cohort_id=h.id),
 h.starts_at,h.enrollment_closes_at
 FROM academy_cohorts h
 JOIN academy_enrollments e ON e.version_id=h.version_id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid
 ORDER BY h.starts_at,h.id LIMIT 100`, enrollmentID, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cohorts := []academy.Cohort{}
	for rows.Next() {
		var item academy.Cohort
		var starts, closes sql.NullTime
		if err = rows.Scan(&item.ID, &item.VersionID, &item.Label, &item.Capacity, &item.Members, &starts, &closes); err != nil {
			return nil, err
		}
		if starts.Valid {
			item.StartsAt = &starts.Time
		}
		if closes.Valid {
			item.EnrollmentClosesAt = &closes.Time
		}
		cohorts = append(cohorts, item)
	}
	return cohorts, rows.Err()
}
