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
	// A deployment flag and verified file metadata do not establish that an
	// actual learner can finish a protected VIDEO lesson. The current R8
	// runtime has no durable, provider-verified playback completion evidence
	// or independently certified release admission. Never turn sellability
	// on merely because an operator toggled the environment variable.
	//
	// Re-enable only when the canonical Academy/Entitlement path stores and
	// verifies a signed playback attestation and the real payment/revocation
	// acceptance has been independently recorded. This is intentionally
	// fail-closed even with a valid author-owned cloud attachment.
	if !academyVideoPlaybackCertificationAvailable() {
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

// academyVideoPlaybackCertificationAvailable remains false until a genuine
// version-bound, provider-verified completion contract is implemented.
// It is deliberately NOT controlled by an environment variable.
func academyVideoPlaybackCertificationAvailable() bool {
	return false
}
