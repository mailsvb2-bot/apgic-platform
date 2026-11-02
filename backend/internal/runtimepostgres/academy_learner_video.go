package runtimepostgres

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyLearnerVideoManifest exposes strictly APGIC-owned IDs and display
// metadata. A client must never receive a provider file path, URL or OAuth
// credential. Every read uses the canonical active enrollment entitlement
// and checks release windows in PostgreSQL on that same request.
func (c *Checker) AcademyLearnerVideoManifest(actor, enrollmentID, lessonID string) ([]academy.LearnerVideo, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	if !academyMediaUUID(actor) || !academyMediaUUID(enrollmentID) || !academyMediaUUID(lessonID) {
		return nil, academy.ErrInvalid
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT media.id::text,media.title,lm.ordinal
 FROM academy_enrollments e
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_course_versions v ON v.id=e.version_id AND v.state='PUBLISHED'
 JOIN academy_courses c ON c.id=v.course_id AND c.state='PUBLISHED'
 JOIN academy_modules m ON m.version_id=v.id
 JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
 JOIN academy_lesson_media lm ON lm.lesson_id=l.id
 JOIN academy_cloud_media media ON media.id=lm.media_id AND media.author_id=c.author_id
 LEFT JOIN academy_lesson_schedules sc ON sc.lesson_id=l.id
 LEFT JOIN academy_assessment_limits limits ON limits.lesson_id=l.id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid AND l.id=$3::uuid
 AND e.state IN ('ACTIVE','COMPLETED') AND academy_entitlement_is_active(t.id)
 AND `+academyAssessmentOpenSQL+`
 ORDER BY lm.ordinal`, enrollmentID, actor, lessonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []academy.LearnerVideo{}
	for rows.Next() {
		var item academy.LearnerVideo
		if err = rows.Scan(&item.ID, &item.Title, &item.Ordinal); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, academy.ErrEntitlement
	}
	return out, nil
}
