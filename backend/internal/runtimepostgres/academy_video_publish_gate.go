package runtimepostgres

import (
	"context"
	"database/sql"
	"os"
)

func academyVideoPublishEnabled() bool {
	return os.Getenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED") == "true"
}

// Video publishing remains disabled by default until the actual staging
// author -> learner -> revoke journey is proven and deployment is opted in.
// All publication gates run under the existing AcademyPublish transaction.
func academyVideoPublicationReady(ctx context.Context, tx *sql.Tx, author, versionID string) (bool, error) {
	if !academyVideoPublishEnabled() {
		return false, nil
	}
	var ready bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
   SELECT 1 FROM academy_cloud_connections cc
   WHERE cc.author_id=$1::uuid AND cc.provider='YANDEX_DISK'
 ) AND NOT EXISTS (
   SELECT 1 FROM academy_lessons l
   JOIN academy_modules m ON m.id=l.module_id
   WHERE m.version_id=$2::uuid AND l.kind='VIDEO' AND NOT EXISTS (
     SELECT 1 FROM academy_lesson_media lm
     JOIN academy_cloud_media cm ON cm.id=lm.media_id
     WHERE lm.lesson_id=l.id AND cm.author_id=$1::uuid
       AND cm.provider='YANDEX_DISK'
       AND cm.provider_file_ref LIKE 'disk:/%'
   )
 ) AND NOT EXISTS (
   SELECT 1 FROM academy_lesson_media lm
   JOIN academy_lessons l ON l.id=lm.lesson_id
   JOIN academy_modules m ON m.id=l.module_id
   JOIN academy_cloud_media cm ON cm.id=lm.media_id
   WHERE m.version_id=$2::uuid AND l.kind='VIDEO'
     AND (cm.author_id<>$1::uuid OR cm.provider<>'YANDEX_DISK')
 )`, author, versionID).Scan(&ready)
	return ready, err
}
