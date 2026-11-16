package runtimepostgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
)

func academyVideoPublishEnabled() bool {
	return os.Getenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED") == "true"
}

// academyReleaseSourceSHA binds a provider acceptance to the exact deployed
// source tree. Missing or malformed build identity always keeps VIDEO closed.
func academyReleaseSourceSHA() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("APGIC_RELEASE_SOURCE_SHA")))
	if len(value) != 40 {
		return ""
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return ""
		}
	}
	return value
}

// Video publishing remains disabled by default until the actual staging
// author -> learner -> revoke journey is proven and deployment is opted in.
// All publication gates run under the existing AcademyPublish transaction.
func academyVideoPublicationReady(ctx context.Context, tx *sql.Tx, author, versionID string) (bool, error) {
	if !academyVideoPublishEnabled() {
		return false, nil
	}
	// The environment switch is only an operator opt-in. A separate immutable
	// release certification must name an active CDN_PROVIDER instance, its
	// trusted signing key, exact tested source SHA and acceptance evidence.
	// No environment flag can manufacture that durable provider proof.
	sourceSHA := academyReleaseSourceSHA()
	if sourceSHA == "" {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('apgic.release_source_sha',$1,true)`, sourceSHA); err != nil {
		return false, err
	}
	var certified bool
	if err := tx.QueryRowContext(ctx, `SELECT academy_video_provider_certification_available($1)`, sourceSHA).Scan(&certified); err != nil {
		return false, err
	}
	if !certified {
		return false, nil
	}
	var ready bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
   SELECT 1 FROM academy_cloud_connections cc
   WHERE cc.author_id=$1::uuid AND cc.provider='YANDEX_DISK'
 ) AND NOT EXISTS (
   -- Every VIDEO lesson must have a playable asset from the current
   -- OAuth connection, not merely a stale disk:/ path.
   SELECT 1 FROM academy_lessons l
   JOIN academy_modules m ON m.id=l.module_id
   WHERE m.version_id=$2::uuid AND l.kind='VIDEO' AND NOT EXISTS (
     SELECT 1 FROM academy_lesson_media lm
     JOIN academy_cloud_media cm ON cm.id=lm.media_id
     JOIN academy_cloud_connections cc
       ON cc.author_id=cm.author_id AND cc.provider=cm.provider
       AND cc.generation=cm.connection_generation
     WHERE lm.lesson_id=l.id AND cm.author_id=$1::uuid
       AND cm.provider='YANDEX_DISK'
       AND cm.provider_file_ref LIKE 'disk:/%'
       AND cm.resource_id IS NOT NULL AND cm.sha256 IS NOT NULL
       AND cm.file_size>0
   )
 ) AND NOT EXISTS (
   -- No attached VIDEO asset may be unverified, foreign, or from a
   -- disconnected/reconnected account, even if another asset is valid.
   SELECT 1 FROM academy_lesson_media lm
   JOIN academy_lessons l ON l.id=lm.lesson_id
   JOIN academy_modules m ON m.id=l.module_id
   JOIN academy_cloud_media cm ON cm.id=lm.media_id
   LEFT JOIN academy_cloud_connections cc
     ON cc.author_id=cm.author_id AND cc.provider=cm.provider
     AND cc.generation=cm.connection_generation
   WHERE m.version_id=$2::uuid AND l.kind='VIDEO'
     AND (cm.author_id<>$1::uuid OR cm.provider<>'YANDEX_DISK'
       OR cc.generation IS NULL OR cm.resource_id IS NULL
       OR cm.sha256 IS NULL OR cm.file_size IS NULL OR cm.file_size<=0)
 )`, author, versionID).Scan(&ready)
	return ready, err
}
