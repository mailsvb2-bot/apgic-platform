package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The author preview uses the same verified assignment and connection
// generation as learner delivery, without creating an enrollment.
const academyAuthorVideoBinding = ` FROM academy_course_versions v
 JOIN academy_courses c ON c.id=v.course_id
 JOIN academy_modules m ON m.version_id=v.id
 JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
 JOIN academy_lesson_media lm ON lm.lesson_id=l.id
 JOIN academy_cloud_media media ON media.id=lm.media_id AND media.author_id=c.author_id
 JOIN academy_cloud_connections connection ON connection.author_id=c.author_id
 AND connection.provider=media.provider AND connection.generation=media.connection_generation
 WHERE c.author_id=$1::uuid AND v.id=$2::uuid AND l.id=$3::uuid
 AND v.state IN ('DRAFT','PUBLISHED') AND media.provider='YANDEX_DISK'
 AND media.resource_id IS NOT NULL AND media.sha256 IS NOT NULL AND media.file_size>0`

func (c *Checker) AcademyAuthorVideoManifest(actor, versionID, lessonID string) ([]academy.LearnerVideo, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	if !academyMediaUUID(actor) || !academyMediaUUID(versionID) || !academyMediaUUID(lessonID) {
		return nil, academy.ErrInvalid
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT media.id::text,media.title,lm.ordinal`+academyAuthorVideoBinding+` ORDER BY lm.ordinal`, actor, versionID, lessonID)
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

func (c *Checker) AcademyAuthorVideoSource(actor, versionID, lessonID, mediaID string) (string, string, string, error) {
	if c == nil || c.db == nil {
		return "", "", "", academy.ErrUnavailable
	}
	for _, value := range []string{actor, versionID, lessonID, mediaID} {
		if !academyMediaUUID(value) {
			return "", "", "", academy.ErrInvalid
		}
	}
	ctx, cancel := academyContext()
	defer cancel()
	var author, provider, path string
	err := c.db.QueryRowContext(ctx, `SELECT c.author_id::text,media.provider,media.provider_file_ref`+academyAuthorVideoBinding+` AND media.id=$4::uuid`, actor, versionID, lessonID, mediaID).Scan(&author, &provider, &path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", academy.ErrEntitlement
	}
	if err != nil {
		return "", "", "", err
	}
	return author, provider, path, nil
}
