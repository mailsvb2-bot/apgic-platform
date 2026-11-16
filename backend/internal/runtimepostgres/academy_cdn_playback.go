package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type AcademyCDNPlaybackReceipt struct {
	ID          string
	EvidenceRef string
	Duplicate   bool
	CompletedAt time.Time
}

// AcademyRecordCDNPlaybackEvidence is the only Academy write path for a
// versioned CDN completion. The caller must supply a Verified attestation
// produced by the canonical connector verifier; raw JSON is never accepted.
// A completion is recorded only when signed ranges cover the full duration.
func (c *Checker) AcademyRecordCDNPlaybackEvidence(
	grant connector.CDNPlaybackGrant,
	verified connector.VerifiedCDNPlaybackAttestation,
) (AcademyCDNPlaybackReceipt, error) {
	if grant.Validate() != nil || !verified.Valid() ||
		verified.Binding() != grant.Binding ||
		verified.ProviderKind() != grant.ProviderKind ||
		verified.ProviderInstanceID() != grant.ProviderInstanceID ||
		verified.SessionID() != grant.SessionID ||
		!verified.CoversEntireDuration() ||
		verified.CompletedSeconds() <= 0 ||
		time.Now().UTC().After(grant.ExpiresAt.Add(connector.CDNPlaybackClockSkew)) {
		return AcademyCDNPlaybackReceipt{}, academy.ErrInvalid
	}
	binding := verified.Binding()
	attestationJSON, err := verified.AttestationJSON()
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, academy.ErrInvalid
	}
	rangesJSON, err := json.Marshal(verified.WatchedRanges())
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, academy.ErrInvalid
	}
	digest, err := verified.EvidenceSHA256()
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, academy.ErrInvalid
	}
	evidenceRef := "sha256:" + digest

	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}
	defer tx.Rollback()

	// Exact event retries are safe even after a later entitlement revocation:
	// they return the immutable historical receipt but never recreate access.
	if existing, found, lookupErr := academyCDNPlaybackExistingTx(
		ctx, tx, grant, verified, digest,
	); lookupErr != nil {
		return AcademyCDNPlaybackReceipt{}, lookupErr
	} else if found {
		if err := tx.Commit(); err != nil {
			return AcademyCDNPlaybackReceipt{}, err
		}
		return existing, nil
	}
	sourceSHA := academyReleaseSourceSHA()
	if sourceSHA == "" {
		return AcademyCDNPlaybackReceipt{}, academy.ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('apgic.release_source_sha',$1,true)`, sourceSHA); err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}

	var subjectID, entitlementID, versionID, courseID, authorID string
	var contentSHA, providerKind string
	var contentLength int64
	err = tx.QueryRowContext(ctx, `SELECT
 e.identity_id::text,e.entitlement_id::text,e.version_id::text,
 c.id::text,c.author_id::text,cm.sha256,cm.file_size,ci.provider_kind
 FROM academy_enrollments e
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_course_versions v ON v.id=e.version_id AND v.state='PUBLISHED'
 JOIN academy_courses c ON c.id=v.course_id AND c.state='PUBLISHED'
 JOIN academy_modules m ON m.version_id=v.id
 JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
 JOIN academy_lesson_media lm ON lm.lesson_id=l.id AND lm.media_id=$3::uuid
 JOIN academy_cloud_media cm ON cm.id=lm.media_id AND cm.author_id=c.author_id
 JOIN academy_cloud_connections cc
   ON cc.author_id=cm.author_id AND cc.provider=cm.provider
   AND cc.generation=cm.connection_generation
 JOIN connector_instances ci
   ON ci.id=$4::uuid AND ci.capability_class='CDN_PROVIDER' AND ci.status='ACTIVE'
 JOIN academy_video_provider_certifications cert
   ON cert.connector_instance_id=ci.id
   AND cert.contract_version=$5 AND cert.provider_kind=ci.provider_kind
   AND cert.key_id=$6 AND cert.tested_source_sha=$7 AND cert.expires_at>now()
 WHERE e.id=$1::uuid AND e.identity_id=$8::uuid AND l.id=$2::uuid
 AND e.state='ACTIVE' AND academy_entitlement_is_active(t.id)
 AND cm.provider='YANDEX_DISK' AND cm.resource_id IS NOT NULL
 AND cm.sha256 IS NOT NULL AND cm.file_size>0
 FOR UPDATE OF e,t,ci`,
		binding.EnrollmentID, binding.LessonID, binding.MediaID,
		grant.ProviderInstanceID, connector.CDNPlaybackContractVersion,
		verified.KeyID(), sourceSHA, binding.SubjectID,
	).Scan(
		&subjectID, &entitlementID, &versionID, &courseID, &authorID,
		&contentSHA, &contentLength, &providerKind,
	)
	if err == sql.ErrNoRows {
		return AcademyCDNPlaybackReceipt{}, academy.ErrEntitlement
	}
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}
	if subjectID != binding.SubjectID || entitlementID != binding.EntitlementID ||
		versionID != binding.CourseVersionID || courseID != binding.CourseID ||
		contentSHA != binding.ContentSHA256 || contentLength != binding.ContentLength ||
		providerKind != grant.ProviderKind || authorID == "" {
		return AcademyCDNPlaybackReceipt{}, academy.ErrEntitlement
	}

	id, err := academyID()
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}
	watchedSeconds := verified.CompletedSeconds()
	requiredSeconds := (verified.DurationMilliseconds() + 999) / 1000
	if watchedSeconds < requiredSeconds {
		return AcademyCDNPlaybackReceipt{}, academy.ErrState
	}
	var insertedID string
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_media_playback_facts (
 id,enrollment_id,lesson_id,provider_event_id,provider_instance_id,evidence_ref,
 watched_seconds,required_seconds,occurred_at,contract_version,course_id,
 course_version_id,subject_id,entitlement_id,media_id,session_id,provider_kind,
 content_sha256,content_length,duration_millis,watched_ranges,signed_attestation,
 attestation_sha256,key_id
) VALUES (
 $1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,$7,$8,$9,$10,$11::uuid,
 $12::uuid,$13::uuid,$14::uuid,$15::uuid,$16::uuid,$17,$18,$19,$20,
 $21::jsonb,$22::jsonb,$23,$24
) ON CONFLICT DO NOTHING
RETURNING id::text`,
		id, binding.EnrollmentID, binding.LessonID, verified.ProviderEventID(),
		grant.ProviderInstanceID, evidenceRef, watchedSeconds, requiredSeconds,
		verified.CompletedAt(), connector.CDNPlaybackContractVersion, binding.CourseID,
		binding.CourseVersionID, binding.SubjectID, binding.EntitlementID,
		binding.MediaID, verified.SessionID(), verified.ProviderKind(),
		binding.ContentSHA256, binding.ContentLength, verified.DurationMilliseconds(),
		string(rangesJSON), string(attestationJSON), digest, verified.KeyID(),
	).Scan(&insertedID)
	if err == sql.ErrNoRows {
		existing, found, lookupErr := academyCDNPlaybackExistingTx(ctx, tx, grant, verified, digest)
		if lookupErr != nil {
			return AcademyCDNPlaybackReceipt{}, lookupErr
		}
		if !found {
			return AcademyCDNPlaybackReceipt{}, academy.ErrConflict
		}
		if err := tx.Commit(); err != nil {
			return AcademyCDNPlaybackReceipt{}, err
		}
		return existing, nil
	}
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}

	var completedLesson string
	progressErr := tx.QueryRowContext(ctx, `INSERT INTO academy_progress(
 enrollment_id,lesson_id,evidence_ref,completed_at
) VALUES($1::uuid,$2::uuid,$3,$4)
ON CONFLICT(enrollment_id,lesson_id) DO NOTHING
RETURNING lesson_id::text`,
		binding.EnrollmentID, binding.LessonID, evidenceRef, verified.CompletedAt(),
	).Scan(&completedLesson)
	if progressErr == nil {
		if err := academyEvent(ctx, tx, binding.SubjectID,
			"academy/enrollment/"+binding.EnrollmentID, "academy.lesson.completed",
			map[string]any{
				"enrollment_id": binding.EnrollmentID,
				"lesson_id": binding.LessonID,
				"media_id": binding.MediaID,
				"provider_instance_id": grant.ProviderInstanceID,
				"provider_event_id": verified.ProviderEventID(),
				"evidence_ref": evidenceRef,
				"watched_seconds": watchedSeconds,
				"required_seconds": requiredSeconds,
			}); err != nil {
			return AcademyCDNPlaybackReceipt{}, err
		}
	} else if progressErr != sql.ErrNoRows {
		return AcademyCDNPlaybackReceipt{}, progressErr
	}
	if err := tx.Commit(); err != nil {
		return AcademyCDNPlaybackReceipt{}, err
	}
	return AcademyCDNPlaybackReceipt{
		ID: insertedID, EvidenceRef: evidenceRef, CompletedAt: verified.CompletedAt(),
	}, nil
}

func academyCDNPlaybackExistingTx(
	ctx context.Context,
	tx *sql.Tx,
	grant connector.CDNPlaybackGrant,
	verified connector.VerifiedCDNPlaybackAttestation,
	digest string,
) (AcademyCDNPlaybackReceipt, bool, error) {
	binding := verified.Binding()
	var id, storedDigest, storedEventID, subjectID, entitlementID, versionID, courseID string
	var lessonID, mediaID, sessionID, providerKind, contentSHA, keyID string
	var contentLength int64
	err := tx.QueryRowContext(ctx, `SELECT id::text,attestation_sha256,provider_event_id,
 subject_id::text,entitlement_id::text,course_version_id::text,course_id::text,
 lesson_id::text,media_id::text,session_id::text,provider_kind,content_sha256,
 content_length,key_id
 FROM academy_media_playback_facts
 WHERE provider_instance_id=$1::uuid AND (provider_event_id=$2 OR session_id=$3::uuid)
 FOR SHARE`,
		grant.ProviderInstanceID, verified.ProviderEventID(), verified.SessionID(),
	).Scan(&id, &storedDigest, &storedEventID, &subjectID, &entitlementID, &versionID, &courseID,
		&lessonID, &mediaID, &sessionID, &providerKind, &contentSHA, &contentLength, &keyID)
	if err == sql.ErrNoRows {
		return AcademyCDNPlaybackReceipt{}, false, nil
	}
	if err != nil {
		return AcademyCDNPlaybackReceipt{}, false, err
	}
	if storedEventID != verified.ProviderEventID() || storedDigest != digest || subjectID != binding.SubjectID ||
		entitlementID != binding.EntitlementID || versionID != binding.CourseVersionID ||
		courseID != binding.CourseID || lessonID != binding.LessonID ||
		mediaID != binding.MediaID || sessionID != verified.SessionID() ||
		providerKind != verified.ProviderKind() || contentSHA != binding.ContentSHA256 ||
		contentLength != binding.ContentLength || keyID != verified.KeyID() {
		return AcademyCDNPlaybackReceipt{}, false, academy.ErrConflict
	}
	return AcademyCDNPlaybackReceipt{
		ID: id, EvidenceRef: "sha256:" + digest, Duplicate: true,
		CompletedAt: verified.CompletedAt(),
	}, true, nil
}
