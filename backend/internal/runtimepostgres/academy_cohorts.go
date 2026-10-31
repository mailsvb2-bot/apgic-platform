package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type AcademyCohort = academy.Cohort

func (c *Checker) AcademyListCohorts(actor string) ([]AcademyCohort, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), academyTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT h.id::text,h.version_id::text,h.title,h.capacity,
  (SELECT count(*) FROM academy_cohort_memberships m WHERE m.cohort_id=h.id),
  h.starts_at,h.enrollment_closes_at
  FROM academy_cohorts h JOIN academy_course_versions v ON v.id=h.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE c.author_id=$1::uuid ORDER BY h.starts_at,h.id`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AcademyCohort{}
	for rows.Next() {
		var x AcademyCohort
		var starts, closes sql.NullTime
		if err = rows.Scan(&x.ID, &x.VersionID, &x.Label, &x.Capacity, &x.Members, &starts, &closes); err != nil {
			return nil, err
		}
		if starts.Valid {
			x.StartsAt = &starts.Time
		}
		if closes.Valid {
			x.EnrollmentClosesAt = &closes.Time
		}
		result = append(result, x)
	}
	return result, rows.Err()
}

func (c *Checker) AcademyCreateCohort(actor, versionID, label string, capacity int, startsAt, enrollmentClosesAt *time.Time) (AcademyCohort, error) {
	label = strings.TrimSpace(label)
	if actor == "" || versionID == "" || len(label) < 1 || len(label) > 160 || capacity < 1 || capacity > 100000 {
		return AcademyCohort{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return AcademyCohort{}, err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT v.state FROM academy_course_versions v
 JOIN academy_courses c ON c.id=v.course_id WHERE v.id=$1::uuid AND c.author_id=$2::uuid FOR SHARE OF v`, versionID, actor).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return AcademyCohort{}, academy.ErrNotFound
	}
	if err != nil {
		return AcademyCohort{}, err
	}
	if state != "PUBLISHED" {
		return AcademyCohort{}, academy.ErrState
	}
	if startsAt == nil || !startsAt.After(time.Now().UTC()) || (enrollmentClosesAt != nil && (enrollmentClosesAt.After(*startsAt) || !enrollmentClosesAt.After(time.Now().UTC()))) {
		return AcademyCohort{}, academy.ErrInvalid
	}
	id, err := academyID()
	if err != nil {
		return AcademyCohort{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_cohorts(id,version_id,title,capacity,starts_at,enrollment_closes_at)
 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6)`, id, versionID, label, capacity, startsAt, enrollmentClosesAt)
	if err != nil {
		return AcademyCohort{}, fmt.Errorf("create academy cohort: %w", err)
	}
	if err = academyEvent(ctx, tx, actor, "academy/cohort/"+id, "academy.cohort.created", map[string]any{"version_id": versionID, "capacity": capacity}); err != nil {
		return AcademyCohort{}, err
	}
	if err = tx.Commit(); err != nil {
		return AcademyCohort{}, err
	}
	return AcademyCohort{ID: id, VersionID: versionID, Label: label, Capacity: capacity, StartsAt: startsAt, EnrollmentClosesAt: enrollmentClosesAt}, nil
}

func (c *Checker) AcademyJoinCohort(actor, cohortID, enrollmentID string) error {
	if actor == "" || cohortID == "" || enrollmentID == "" {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Lock the canonical cohort row for capacity and schedule decisions.
	// The SQL trigger independently enforces these invariants on direct writes.
	var state string
	var entitlementActive, memberAlready, otherMembership, prerequisitesComplete bool
	var capacity sql.NullInt64
	var startsAt time.Time
	var closesAt sql.NullTime
	var members int
	err = tx.QueryRowContext(ctx, `SELECT e.state, academy_entitlement_is_active(e.entitlement_id),
 h.capacity, h.starts_at, h.enrollment_closes_at,
 (SELECT count(*) FROM academy_cohort_memberships m WHERE m.cohort_id=h.id),
 EXISTS(SELECT 1 FROM academy_cohort_memberships m WHERE m.cohort_id=h.id AND m.enrollment_id=e.id),
 EXISTS(SELECT 1 FROM academy_cohort_memberships m WHERE m.enrollment_id=e.id AND m.cohort_id<>h.id),
 NOT EXISTS(SELECT 1 FROM academy_prerequisites p WHERE p.version_id=h.version_id
   AND NOT EXISTS(SELECT 1 FROM academy_enrollments prior
     WHERE prior.identity_id=e.identity_id AND prior.version_id=p.prerequisite_version_id
       AND prior.state='COMPLETED'))
 FROM academy_cohorts h
 JOIN academy_enrollments e ON e.version_id=h.version_id
 WHERE h.id=$1::uuid AND e.id=$2::uuid AND e.identity_id=$3::uuid
 FOR UPDATE OF h,e`, cohortID, enrollmentID, actor).Scan(
		&state, &entitlementActive, &capacity, &startsAt, &closesAt,
		&members, &memberAlready, &otherMembership, &prerequisitesComplete)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrEntitlement
	}
	if err != nil {
		return err
	}
	if state != "ACTIVE" || !entitlementActive {
		return academy.ErrEntitlement
	}
	if memberAlready {
		return tx.Commit()
	}
	if otherMembership {
		return academy.ErrConflict
	}
	policy := academy.CohortPolicy{
		Enrolled: members,
		StartsAt: startsAt,
		PrerequisitesRequired: !prerequisitesComplete,
	}
	if capacity.Valid {
		policy.Capacity = int(capacity.Int64)
	}
	if closesAt.Valid {
		policy.EnrollmentClosesAt = closesAt.Time
	}
	if err = academy.AuthorizeCohortAdmission(policy, academy.CohortAdmissionFacts{
		Now: time.Now().UTC(), EntitlementActive: entitlementActive,
		PrerequisitesComplete: prerequisitesComplete,
	}); err != nil {
		return academy.ErrState
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_cohort_memberships(cohort_id,enrollment_id)
 VALUES($1::uuid,$2::uuid)`, cohortID, enrollmentID)
	if err != nil {
		return fmt.Errorf("academy cohort admission: %w", err)
	}
	if err = academyEvent(ctx, tx, actor, "academy/cohort/"+cohortID,
		"academy.cohort.joined", map[string]string{"enrollment_id": enrollmentID}); err != nil {
		return err
	}
	return tx.Commit()
}
