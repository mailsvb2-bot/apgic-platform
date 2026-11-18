package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyOriginalRightsStatus reads the already attested exact-version
// curriculum. Author-only; old attestations cannot authorize changed lessons.
func (c *Checker) AcademyOriginalRightsStatus(actor, courseID, versionID string) (bool, string, error) {
	if !academyMediaUUID(actor) || !academyMediaUUID(courseID) || !academyMediaUUID(versionID) {
		return false, "", academy.ErrInvalid
	}
	if c == nil || c.db == nil {
		return false, "", academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var owner bool
	if err := c.db.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM academy_courses c
 WHERE c.id=$2::uuid AND c.author_id=$1::uuid)`, actor, courseID).Scan(&owner); err != nil {
		return false, "", err
	}
	if !owner {
		return false, "", academy.ErrDenied
	}
	var state string
	err := c.db.QueryRowContext(ctx, `SELECT state FROM academy_course_versions
 WHERE id=$1::uuid AND course_id=$2::uuid`, versionID, courseID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", academy.ErrNotFound
	}
	if err != nil {
		return false, "", err
	}
	var provenance string
	err = c.db.QueryRowContext(ctx, `SELECT provenance_ref FROM academy_original_rights
 WHERE version_id=$1::uuid AND author_id=$2::uuid
 AND curriculum_sha256=academy_curriculum_fingerprint($1::uuid)
 ORDER BY attested_at DESC LIMIT 1`, versionID, actor).Scan(&provenance)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, provenance, nil
}
