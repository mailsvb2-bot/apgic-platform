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

type AcademyCohort struct {
	ID                 string     `json:"id"`
	VersionID          string     `json:"version_id"`
	Label              string     `json:"label"`
	Capacity           int        `json:"capacity"`
	Members            int        `json:"members"`
	StartsAt           *time.Time `json:"starts_at,omitempty"`
	EnrollmentClosesAt *time.Time `json:"enrollment_closes_at,omitempty"`
}

func (c *Checker) AcademyListCohorts(actor string) ([]AcademyCohort, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), academyTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT h.id::text,h.version_id::text,h.label,h.capacity,
  (SELECT count(*) FROM academy_cohort_memberships m WHERE m.cohort_id=h.id),
  h.starts_at,h.enrollment_closes_at
  FROM academy_cohorts h JOIN academy_course_versions v ON v.id=h.version_id
  JOIN academy_courses c ON c.id=v.course_id
  WHERE c.author_id=$1::uuid ORDER BY h.created_at,h.id`, actor)
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
	if startsAt != nil && enrollmentClosesAt != nil && !enrollmentClosesAt.Before(*startsAt) {
		return AcademyCohort{}, academy.ErrInvalid
	}
	id, err := academyID()
	if err != nil {
		return AcademyCohort{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_cohorts(id,version_id,label,capacity,starts_at,enrollment_closes_at)
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
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academy_enrollments e
 JOIN academy_cohorts h ON h.version_id=e.version_id
 WHERE h.id=$1::uuid AND e.id=$2::uuid AND e.identity_id=$3::uuid
 AND e.state='ACTIVE' AND academy_entitlement_is_active(e.entitlement_id))`, cohortID, enrollmentID, actor).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return academy.ErrEntitlement
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO academy_cohort_memberships(cohort_id,enrollment_id)
 VALUES($1::uuid,$2::uuid) ON CONFLICT DO NOTHING`, cohortID, enrollmentID)
	if err != nil {
		return fmt.Errorf("academy cohort admission: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed > 0 {
		if err = academyEvent(ctx, tx, actor, "academy/cohort/"+cohortID, "academy.cohort.joined", map[string]string{"enrollment_id": enrollmentID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
