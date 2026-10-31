package runtimepostgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const academyTimeout = 5 * time.Second

func academyID() (string, error) { return persistentid.New() }
func academyToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func academyContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), academyTimeout)
}

func academyEvent(ctx context.Context, tx *sql.Tx, actor, resource, eventType string, payload any) error {
	id, err := academyID()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The common event spine must observe a non-null, strictly ordered version.
	// Lock the aggregate for the whole transaction before assigning its next
	// sequence; multiple lesson submissions can otherwise race each other.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7319))`, resource); err != nil {
		return fmt.Errorf("academy outbox ordering lock: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events
 (event_id,event_type,schema_version,aggregate_ref,aggregate_version,correlation_id,idempotency_key,occurred_at,producer,payload)
 VALUES ($1::uuid,$2,'academy-event-v1',$3,
 (SELECT COALESCE(MAX(aggregate_version),0)+1 FROM outbox_events WHERE aggregate_ref=$3),
 $1,$2||'/'||$3||'/'||$1::text,now(),'apgic-academy',$4::jsonb)`,
		id, eventType, resource, string(raw))
	if err != nil {
		return fmt.Errorf("academy outbox: %w", err)
	}
	// A bounded event contains only identity references and state, never lesson content.
	return nil
}
func academyBegin(c *Checker) (context.Context, context.CancelFunc, *sql.Tx, error) {
	ctx, cancel := academyContext()
	if c == nil || c.db == nil {
		return ctx, cancel, nil, academy.ErrUnavailable
	}
	tx, err := c.db.BeginTx(ctx, nil)
	return ctx, cancel, tx, err
}
func academyAuthor(ctx context.Context, tx *sql.Tx, actor, courseID string) (bool, error) {
	var author string
	err := tx.QueryRowContext(ctx, `SELECT author_id::text FROM academy_courses WHERE id=$1::uuid FOR UPDATE`, courseID).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return false, academy.ErrNotFound
	}
	return author == actor, err
}
func academyPublishedProductOwner(ctx context.Context, tx *sql.Tx, actor, productID string) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
  SELECT 1 FROM products p WHERE p.id=$1::uuid AND p.status='PUBLISHED'
    AND ((p.owner_type='IDENTITY' AND p.owner_id=$2::uuid)
    OR (p.owner_type='ORGANIZATION' AND EXISTS(
      SELECT 1 FROM organization_ownerships o JOIN organization_memberships m
       ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id
      WHERE o.organization_id=p.owner_id AND o.identity_id=$2::uuid
       AND o.status='ACTIVE' AND m.status='ACTIVE'
    )))
 )`, productID, actor).Scan(&allowed)
	return allowed, err
}
func (c *Checker) AcademyCreateCourse(actor string, input academy.Course) (academy.Course, error) {
	input.Title = strings.TrimSpace(input.Title)
	if actor == "" || len(input.Title) < 3 || len(input.Title) > 200 || input.ProductID == "" {
		return academy.Course{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Course{}, err
	}
	defer tx.Rollback()
	owner, err := academyPublishedProductOwner(ctx, tx, actor, input.ProductID)
	if err != nil {
		return academy.Course{}, err
	}
	if !owner {
		return academy.Course{}, academy.ErrDenied
	}
	courseID, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	versionID, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_courses(id,product_id,author_id,title,description)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)`, courseID, input.ProductID, actor, input.Title, input.Description)
	if err != nil {
		return academy.Course{}, fmt.Errorf("create academy course: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_course_versions(id,course_id,version_no)
 VALUES($1::uuid,$2::uuid,1)`, versionID, courseID)
	if err != nil {
		return academy.Course{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/course/"+courseID, "academy.course.created", map[string]string{"author_id": actor, "version_id": versionID}); err != nil {
		return academy.Course{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Course{}, err
	}
	return academy.Course{ID: courseID, ProductID: input.ProductID, Title: input.Title, Description: input.Description, AuthorID: actor, State: "DRAFT", VersionID: versionID, VersionNo: 1}, nil
}
func (c *Checker) AcademyNewVersion(actor, courseID string) (academy.Course, error) {
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Course{}, err
	}
	defer tx.Rollback()
	owner, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.Course{}, err
	}
	if !owner {
		return academy.Course{}, academy.ErrDenied
	}
	var draftExists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academy_course_versions WHERE course_id=$1::uuid AND state='DRAFT')`, courseID).Scan(&draftExists)
	if err != nil {
		return academy.Course{}, err
	}
	if draftExists {
		return academy.Course{}, academy.ErrState
	}
	var no int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_no),0)+1 FROM academy_course_versions WHERE course_id=$1::uuid`, courseID).Scan(&no)
	if err != nil {
		return academy.Course{}, err
	}
	id, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_course_versions(id,course_id,version_no) VALUES($1::uuid,$2::uuid,$3)`, id, courseID, no)
	if err != nil {
		return academy.Course{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/course/"+courseID, "academy.version.drafted", map[string]any{"version_id": id, "version_no": no}); err != nil {
		return academy.Course{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Course{}, err
	}
	return academy.Course{ID: courseID, VersionID: id, VersionNo: no, State: "DRAFT"}, nil
}
func (c *Checker) AcademyAddModule(actor, courseID, versionID string, m academy.Module) (academy.Module, error) {
	if m.Ordinal < 1 || strings.TrimSpace(m.Title) == "" {
		return academy.Module{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Module{}, err
	}
	defer tx.Rollback()
	owner, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.Module{}, err
	}
	if !owner {
		return academy.Module{}, academy.ErrDenied
	}
	var state string
	err = tx.QueryRowContext(ctx, `SELECT state FROM academy_course_versions WHERE id=$1::uuid AND course_id=$2::uuid FOR UPDATE`, versionID, courseID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Module{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Module{}, err
	}
	if state != "DRAFT" {
		return academy.Module{}, academy.ErrState
	}
	id, err := academyID()
	if err != nil {
		return academy.Module{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_modules(id,version_id,ordinal,title)VALUES($1::uuid,$2::uuid,$3,$4)`, id, versionID, m.Ordinal, strings.TrimSpace(m.Title))
	if err != nil {
		return academy.Module{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/version/"+versionID, "academy.module.added", map[string]any{"module_id": id, "ordinal": m.Ordinal}); err != nil {
		return academy.Module{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Module{}, err
	}
	m.ID = id
	m.VersionID = versionID
	return m, nil
}
func (c *Checker) AcademyAddLesson(actor, courseID, versionID, moduleID string, l academy.Lesson) (academy.Lesson, error) {
	if err := academy.ValidateLesson(l); err != nil {
		return academy.Lesson{}, err
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Lesson{}, err
	}
	defer tx.Rollback()
	owner, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.Lesson{}, err
	}
	if !owner {
		return academy.Lesson{}, academy.ErrDenied
	}
	var state string
	err = tx.QueryRowContext(ctx, `SELECT v.state FROM academy_course_versions v
  JOIN academy_modules m ON m.version_id=v.id
  WHERE m.id=$1::uuid AND v.id=$2::uuid AND v.course_id=$3::uuid FOR UPDATE OF v`, moduleID, versionID, courseID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Lesson{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Lesson{}, err
	}
	if state != "DRAFT" {
		return academy.Lesson{}, academy.ErrState
	}
	id, err := academyID()
	if err != nil {
		return academy.Lesson{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_lessons(id,module_id,ordinal,title,kind,body,media_asset_ref)
 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,NULLIF($7,''))`, id, moduleID, l.Ordinal, strings.TrimSpace(l.Title), string(l.Kind), l.Body, l.MediaAssetRef)
	if err != nil {
		return academy.Lesson{}, err
	}
	if l.Kind == academy.Assessment {
		_, err = tx.ExecContext(ctx, `INSERT INTO academy_assessments(lesson_id,passing_score,rubric_version)
  VALUES($1::uuid,$2,$3)`, id, l.PassingScore, l.RubricVersion)
		if err != nil {
			return academy.Lesson{}, err
		}
	}
	if err = academyEvent(ctx, tx, actor, "academy/version/"+versionID, "academy.lesson.added", map[string]any{"lesson_id": id, "kind": l.Kind}); err != nil {
		return academy.Lesson{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Lesson{}, err
	}
	l.ID = id
	l.ModuleID = moduleID
	return l, nil
}

// AcademyDeclareOriginalRights signs a snapshot of an ORIGINAL-work curriculum.
func (c *Checker) AcademyDeclareOriginalRights(actor, courseID, versionID, provenanceRef string) (academy.OriginalRightsDeclaration, error) {
	provenanceRef = strings.TrimSpace(provenanceRef)
	if len(provenanceRef) < 12 || len(provenanceRef) > 2048 {
		return academy.OriginalRightsDeclaration{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	defer tx.Rollback()
	isAuthor, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	if !isAuthor {
		return academy.OriginalRightsDeclaration{}, academy.ErrDenied
	}
	var state, fingerprint string
	err = tx.QueryRowContext(ctx, `SELECT state,academy_curriculum_fingerprint(id)
 FROM academy_course_versions WHERE course_id=$1::uuid AND id=$2::uuid FOR UPDATE`,
		courseID, versionID).Scan(&state, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.OriginalRightsDeclaration{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	if state != "DRAFT" {
		return academy.OriginalRightsDeclaration{}, academy.ErrState
	}
	// A retry of the signed acknowledgement must return the same immutable
	// evidence, not issue another record or report a misleading 503.
	var existing academy.OriginalRightsDeclaration
	err = tx.QueryRowContext(ctx, `SELECT id::text,provenance_ref,policy_version,attested_at
 FROM academy_original_rights WHERE version_id=$1::uuid
 AND author_id=$2::uuid AND curriculum_sha256=$3`,
		versionID, actor, fingerprint).Scan(&existing.ID, &existing.ProvenanceRef,
		&existing.PolicyVersion, &existing.AttestedAt)
	if err == nil {
		existing.VersionID = versionID
		existing.CurriculumSHA256 = fingerprint
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return academy.OriginalRightsDeclaration{}, err
	}
	id, err := academyID()
	if err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	var attestedAt time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_original_rights
 (id,version_id,author_id,rights_basis,provenance_ref,policy_version,curriculum_sha256)
 VALUES ($1::uuid,$2::uuid,$3::uuid,'ORIGINAL_AUTHOR_DECLARATION',$4,
 'academy-original-rights-v1',$5) RETURNING attested_at`,
		id, versionID, actor, provenanceRef, fingerprint).Scan(&attestedAt)
	if err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/version/"+versionID,
		"academy.original_rights.attested", map[string]string{
			"version_id": versionID, "curriculum_sha256": fingerprint,
		}); err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.OriginalRightsDeclaration{}, err
	}
	return academy.OriginalRightsDeclaration{
		ID: id, VersionID: versionID, CurriculumSHA256: fingerprint,
		PolicyVersion: "academy-original-rights-v1", ProvenanceRef: provenanceRef,
		AttestedAt: attestedAt,
	}, nil
}

func (c *Checker) AcademyPublish(actor, courseID, versionID string) (academy.Course, error) {
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Course{}, err
	}
	defer tx.Rollback()
	owner, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.Course{}, err
	}
	if !owner {
		return academy.Course{}, academy.ErrDenied
	}
	var pID, title, description, state string
	var no int
	err = tx.QueryRowContext(ctx, `SELECT c.product_id::text,c.title,c.description,v.state,v.version_no
 FROM academy_courses c JOIN academy_course_versions v ON v.course_id=c.id
 WHERE c.id=$1::uuid AND v.id=$2::uuid FOR UPDATE OF v`, courseID, versionID).Scan(&pID, &title, &description, &state, &no)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Course{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Course{}, err
	}
	if state != "DRAFT" {
		return academy.Course{}, academy.ErrState
	}
	// Until a trusted media-playback connector can attest completion, publishing
	// VIDEO lessons would sell an unfinishable course. Keep draft authoring only.
	var unsupportedVideo bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
  SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
  WHERE m.version_id=$1::uuid AND l.kind='VIDEO'
 )`, versionID).Scan(&unsupportedVideo)
	if err != nil {
		return academy.Course{}, err
	}
	if unsupportedVideo {
		return academy.Course{}, academy.ErrState
	}
	var attested bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academy_original_rights
  WHERE version_id=$1::uuid AND author_id=$2::uuid
   AND curriculum_sha256=academy_curriculum_fingerprint($1::uuid))`, versionID, actor).Scan(&attested)
	if err != nil {
		return academy.Course{}, err
	}
	if !attested {
		return academy.Course{}, academy.ErrState
	}
	allowed, err := academyPublishedProductOwner(ctx, tx, actor, pID)
	if err != nil {
		return academy.Course{}, err
	}
	if !allowed {
		return academy.Course{}, academy.ErrDenied
	}
	result, err := tx.ExecContext(ctx, `UPDATE academy_course_versions SET state='PUBLISHED',published_at=now(),title_snapshot=$2,description_snapshot=$3
 WHERE id=$1::uuid AND state='DRAFT' AND EXISTS (
 SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id WHERE m.version_id=$1::uuid)`, versionID, title, description)
	if err != nil {
		return academy.Course{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return academy.Course{}, err
	}
	if n != 1 {
		return academy.Course{}, academy.ErrState
	}
	if _, err = tx.ExecContext(ctx, `UPDATE academy_courses SET state='PUBLISHED' WHERE id=$1::uuid`, courseID); err != nil {
		return academy.Course{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/course/"+courseID, "academy.version.published", map[string]any{"version_id": versionID, "version_no": no}); err != nil {
		return academy.Course{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Course{}, err
	}
	return academy.Course{ID: courseID, ProductID: pID, Title: title, Description: description, AuthorID: actor, VersionID: versionID, VersionNo: no, State: "PUBLISHED"}, nil
}
func (c *Checker) AcademyCatalog() ([]academy.Course, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT c.id::text,c.product_id::text,c.title,c.description,v.id::text,v.version_no
 FROM academy_courses c JOIN LATERAL (
  SELECT id,version_no FROM academy_course_versions
  WHERE course_id=c.id AND state='PUBLISHED' ORDER BY version_no DESC LIMIT 1
 )v ON true WHERE c.state='PUBLISHED' ORDER BY c.title,c.id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	courses := []academy.Course{}
	for rows.Next() {
		var co academy.Course
		if err = rows.Scan(&co.ID, &co.ProductID, &co.Title, &co.Description, &co.VersionID, &co.VersionNo); err != nil {
			return nil, err
		}
		co.State = "PUBLISHED"
		courses = append(courses, co)
	}
	return courses, rows.Err()
}
func (c *Checker) AcademyEnroll(actor, courseID string) (academy.Enrollment, error) {
	if actor == "" {
		return academy.Enrollment{}, academy.ErrDenied
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Enrollment{}, err
	}
	defer tx.Rollback()
	// Serialize two enroll requests from the same learner and course. On replay,
	// read the already persisted entitlement rather than inventing a new basis.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1),hashtext($2))", actor, courseID); err != nil {
		return academy.Enrollment{}, err
	}
	var versionID, entitlementID string
	err = tx.QueryRowContext(ctx, `SELECT v.id::text,t.id::text
 FROM academy_courses c
 JOIN academy_course_versions v ON v.course_id=c.id AND v.state='PUBLISHED'
 JOIN store_entitlements t ON t.product_ref=c.product_id::text
  AND t.identity_id=$2::uuid AND academy_entitlement_is_active(t.id)
 JOIN store_transaction_verifications sv ON sv.id=t.verification_id
  AND sv.identity_id=t.identity_id AND sv.product_ref=t.product_ref
 JOIN orders order_row ON order_row.id=sv.order_id AND order_row.product_id=c.product_id
 JOIN payment_effects paid ON paid.id=sv.payment_effect_id AND paid.effect_kind='CAPTURED'
 WHERE c.id=$1::uuid AND c.state='PUBLISHED'
 ORDER BY v.version_no DESC LIMIT 1`, courseID, actor).Scan(&versionID, &entitlementID)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Enrollment{}, academy.ErrEntitlement
	}
	if err != nil {
		return academy.Enrollment{}, err
	}
	var e academy.Enrollment
	err = tx.QueryRowContext(ctx, `SELECT id::text,state,enrolled_at,completed_at,entitlement_id::text
 FROM academy_enrollments WHERE identity_id=$1::uuid AND version_id=$2::uuid`, actor, versionID).Scan(&e.ID, &e.State, &e.EnrolledAt, &e.CompletedAt, &e.EntitlementID)
	if err == nil {
		e.VersionID = versionID
		e.IdentityID = actor
		e.AccessState = "ACTIVE"
		return e, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return academy.Enrollment{}, err
	}
	id, err := academyID()
	if err != nil {
		return academy.Enrollment{}, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_enrollments(id,identity_id,version_id,entitlement_id)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid) RETURNING enrolled_at`, id, actor, versionID, entitlementID).Scan(&e.EnrolledAt)
	if err != nil {
		return academy.Enrollment{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/enrollment/"+id, "academy.enrollment.created", map[string]string{"version_id": versionID}); err != nil {
		return academy.Enrollment{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Enrollment{}, err
	}
	e.ID = id
	e.IdentityID = actor
	e.VersionID = versionID
	e.EntitlementID = entitlementID
	e.State = "ACTIVE"
	e.AccessState = "ACTIVE"
	return e, nil
}
func (c *Checker) AcademyLearning(actor, enrollmentID string) ([]academy.Lesson, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT l.id::text,l.module_id::text,l.ordinal,l.title,l.kind,
 CASE WHEN (sc.release_at IS NULL OR now()>=sc.release_at) AND (sc.deadline_at IS NULL OR now()<sc.deadline_at) THEN l.body ELSE '' END,
 CASE WHEN (sc.release_at IS NULL OR now()>=sc.release_at) AND (sc.deadline_at IS NULL OR now()<sc.deadline_at) THEN COALESCE(l.media_asset_ref,'') ELSE '' END,COALESCE(a.passing_score,0),COALESCE(a.rubric_version,''),
 (p.enrollment_id IS NOT NULL),
 ((sc.release_at IS NULL OR now()>=sc.release_at) AND (sc.deadline_at IS NULL OR now()<sc.deadline_at))
 FROM academy_enrollments e JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_modules m ON m.version_id=e.version_id
 JOIN academy_lessons l ON l.module_id=m.id
 LEFT JOIN academy_assessments a ON a.lesson_id=l.id
 LEFT JOIN academy_lesson_schedules sc ON sc.lesson_id=l.id
 LEFT JOIN academy_progress p ON p.enrollment_id=e.id AND p.lesson_id=l.id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid
 AND e.state IN ('ACTIVE','COMPLETED') AND academy_entitlement_is_active(t.id)
 ORDER BY m.ordinal,l.ordinal`, enrollmentID, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lessons := []academy.Lesson{}
	for rows.Next() {
		var l academy.Lesson
		if err = rows.Scan(&l.ID, &l.ModuleID, &l.Ordinal, &l.Title, &l.Kind, &l.Body, &l.MediaAssetRef, &l.PassingScore, &l.RubricVersion, &l.Completed, &l.Available); err != nil {
			return nil, err
		}
		// VIDEO references are opaque media asset identifiers, never signed delivery grants.
		lessons = append(lessons, l)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(lessons) == 0 {
		return nil, academy.ErrEntitlement
	}
	return lessons, nil
}
func (c *Checker) AcademyMarkProgress(actor, enrollmentID, lessonID string) error {
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind, versionID, entitlementState, enrollmentState string
	err = tx.QueryRowContext(ctx, `SELECT l.kind,e.version_id::text,CASE WHEN academy_entitlement_is_active(t.id) THEN 'ACTIVE' ELSE 'REVOKED' END,e.state
 FROM academy_enrollments e JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_modules m ON m.version_id=e.version_id
 JOIN academy_lessons l ON l.module_id=m.id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid AND l.id=$3::uuid
 FOR UPDATE OF e`, enrollmentID, actor, lessonID).Scan(&kind, &versionID, &entitlementState, &enrollmentState)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrNotFound
	}
	if err != nil {
		return err
	}
	if entitlementState != "ACTIVE" {
		return academy.ErrEntitlement
	}
	if enrollmentState != "ACTIVE" {
		return academy.ErrState
	}
	var passed bool
	if kind == string(academy.Assessment) {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academy_attempts WHERE enrollment_id=$1::uuid AND lesson_id=$2::uuid AND result='PASSED')`, enrollmentID, lessonID).Scan(&passed)
		if err != nil {
			return err
		}
	}
	if err = academy.CanMarkProgress(academy.LessonKind(kind), passed, false); err != nil {
		return err
	}
	id, err := academyID()
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO academy_progress(enrollment_id,lesson_id,evidence_ref)
 VALUES($1::uuid,$2::uuid,$3) ON CONFLICT DO NOTHING`, enrollmentID, lessonID, "learning/progress/"+id)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 1 {
		if err = academyEvent(ctx, tx, actor, "academy/enrollment/"+enrollmentID, "academy.lesson.completed", map[string]string{"lesson_id": lessonID, "version_id": versionID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (c *Checker) AcademyComplete(actor, enrollmentID string) (academy.Certificate, error) {
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Certificate{}, err
	}
	defer tx.Rollback()
	var versionID, enrollmentState, entitlementState string
	err = tx.QueryRowContext(ctx, `SELECT e.version_id::text,e.state,CASE WHEN academy_entitlement_is_active(t.id) THEN 'ACTIVE' ELSE 'REVOKED' END FROM academy_enrollments e
 JOIN store_entitlements t ON t.id=e.entitlement_id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid FOR UPDATE OF e`, enrollmentID, actor).Scan(&versionID, &enrollmentState, &entitlementState)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Certificate{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Certificate{}, err
	}
	var cert academy.Certificate
	err = tx.QueryRowContext(ctx, `SELECT id::text,verification_token,state,issued_at FROM academy_certificates WHERE enrollment_id=$1::uuid`, enrollmentID).Scan(&cert.ID, &cert.VerificationToken, &cert.State, &cert.IssuedAt)
	if err == nil {
		cert.EnrollmentID = enrollmentID
		cert.VersionID = versionID
		cert.VerificationURL = "/v1/academy/certificates/verify/" + cert.VerificationToken
		cert.EvidenceCandidateState = academy.EvidenceCandidateStatus
		return cert, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return academy.Certificate{}, err
	}
	var total, completed int
	err = tx.QueryRowContext(ctx, `SELECT count(*),count(p.lesson_id)
 FROM academy_modules m JOIN academy_lessons l ON l.module_id=m.id
 LEFT JOIN academy_progress p ON p.lesson_id=l.id AND p.enrollment_id=$1::uuid
 WHERE m.version_id=$2::uuid`, enrollmentID, versionID).Scan(&total, &completed)
	if err != nil {
		return academy.Certificate{}, err
	}
	if err = academy.CanIssueCertificate(total, completed, entitlementState == "ACTIVE", enrollmentState == "ACTIVE"); err != nil {
		return academy.Certificate{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE academy_enrollments SET state='COMPLETED',completed_at=now() WHERE id=$1::uuid`, enrollmentID)
	if err != nil {
		return academy.Certificate{}, err
	}
	certID, err := academyID()
	if err != nil {
		return academy.Certificate{}, err
	}
	token, err := academyToken()
	if err != nil {
		return academy.Certificate{}, err
	}
	evID, err := academyID()
	if err != nil {
		return academy.Certificate{}, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_certificates
 (id,enrollment_id,verification_token,completion_evidence_ref)
 VALUES($1::uuid,$2::uuid,$3,$4) RETURNING issued_at`, certID, enrollmentID, token, "academy/completion/"+evID).Scan(&cert.IssuedAt)
	if err != nil {
		return academy.Certificate{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_evidence_candidates(id,certificate_id,candidate_identity_id)
 VALUES($1::uuid,$2::uuid,$3::uuid)`, evID, certID, actor)
	if err != nil {
		return academy.Certificate{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/enrollment/"+enrollmentID, "academy.learning.completed", map[string]string{"certificate_id": certID, "version_id": versionID, "qualification_state": academy.EvidenceCandidateStatus}); err != nil {
		return academy.Certificate{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Certificate{}, err
	}
	cert.ID = certID
	cert.EnrollmentID = enrollmentID
	cert.VersionID = versionID
	cert.VerificationToken = token
	cert.State = "ACTIVE"
	cert.VerificationURL = "/v1/academy/certificates/verify/" + token
	cert.EvidenceCandidateState = academy.EvidenceCandidateStatus
	return cert, nil
}
func (c *Checker) AcademyVerify(token string) (map[string]any, error) {
	if len(token) < 32 || len(token) > 200 {
		return nil, academy.ErrNotFound
	}
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var state, courseTitle string
	var versionNo int
	var issuedAt time.Time
	err := c.db.QueryRowContext(ctx, `SELECT cert.state,COALESCE(v.title_snapshot,c.title),v.version_no,cert.issued_at FROM academy_certificates cert
 JOIN academy_enrollments e ON e.id=cert.enrollment_id
 JOIN academy_course_versions v ON v.id=e.version_id JOIN academy_courses c ON c.id=v.course_id
 WHERE cert.verification_token=$1`, token).Scan(&state, &courseTitle, &versionNo, &issuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, academy.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"state": state, "course_title": courseTitle, "course_version": versionNo,
		"issued_at": issuedAt, "qualification_status": "NOT_GRANTED"}, nil
}

func (c *Checker) AcademySubmitAttempt(actor, enrollmentID, lessonID, response string) (academy.Attempt, error) {
	response = strings.TrimSpace(response)
	if len(response) < 1 || len(response) > 30000 {
		return academy.Attempt{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Attempt{}, err
	}
	defer tx.Rollback()
	var rubric, state, entitlementState string
	err = tx.QueryRowContext(ctx, `SELECT a.rubric_version,e.state,CASE WHEN academy_entitlement_is_active(t.id) THEN 'ACTIVE' ELSE 'REVOKED' END
 FROM academy_enrollments e JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_modules m ON m.version_id=e.version_id
 JOIN academy_lessons l ON l.module_id=m.id JOIN academy_assessments a ON a.lesson_id=l.id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid AND l.id=$3::uuid
 FOR UPDATE OF e`, enrollmentID, actor, lessonID).Scan(&rubric, &state, &entitlementState)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Attempt{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Attempt{}, err
	}
	if entitlementState != "ACTIVE" {
		return academy.Attempt{}, academy.ErrEntitlement
	}
	if state != "ACTIVE" {
		return academy.Attempt{}, academy.ErrState
	}
	// The enrollment row above is locked until commit. Serialize attempt
	// submission and forbid retakes while human grading is pending or
	// after a passing evaluation. Fail closed against concurrent requests.
	var blockingAttempt bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM academy_attempts
 WHERE enrollment_id=$1::uuid AND lesson_id=$2::uuid
 AND result IN ('PENDING','PASSED'))`, enrollmentID, lessonID).Scan(&blockingAttempt)
	if err != nil {
		return academy.Attempt{}, err
	}
	if err = academy.CanSubmitAssessmentAttempt(blockingAttempt); err != nil {
		return academy.Attempt{}, err
	}
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_no),0)+1 FROM academy_attempts
 WHERE enrollment_id=$1::uuid AND lesson_id=$2::uuid`, enrollmentID, lessonID).Scan(&attempts)
	if err != nil {
		return academy.Attempt{}, err
	}
	id, err := academyID()
	if err != nil {
		return academy.Attempt{}, err
	}
	var submitted time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO academy_attempts(
 id,enrollment_id,lesson_id,attempt_no,response_text,rubric_version)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6)
 RETURNING submitted_at`, id, enrollmentID, lessonID, attempts, response, rubric).Scan(&submitted)
	if err != nil {
		return academy.Attempt{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/enrollment/"+enrollmentID, "academy.assessment.submitted",
		map[string]any{"attempt_id": id, "lesson_id": lessonID, "attempt_no": attempts}); err != nil {
		return academy.Attempt{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Attempt{}, err
	}
	return academy.Attempt{ID: id, EnrollmentID: enrollmentID, LessonID: lessonID, AttemptNo: attempts,
		Result: "PENDING", RubricVersion: rubric, SubmittedAt: submitted}, nil
}

func (c *Checker) AcademyEvaluateAttempt(author, attemptID, evidence string, score int) (academy.Attempt, error) {
	if score < 0 || score > 100 || strings.TrimSpace(evidence) == "" {
		return academy.Attempt{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Attempt{}, err
	}
	defer tx.Rollback()
	var a academy.Attempt
	var status, entitlementState, enrollmentState string
	var pass int
	var owner string
	err = tx.QueryRowContext(ctx, `SELECT a.enrollment_id::text,a.lesson_id::text,a.attempt_no,
 a.rubric_version,a.result,a.submitted_at,assessment.passing_score,
 c.author_id::text,CASE WHEN academy_entitlement_is_active(t.id) THEN 'ACTIVE' ELSE 'REVOKED' END,e.state
 FROM academy_attempts a
 JOIN academy_assessments assessment ON assessment.lesson_id=a.lesson_id
 JOIN academy_enrollments e ON e.id=a.enrollment_id
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_course_versions v ON v.id=e.version_id
 JOIN academy_courses c ON c.id=v.course_id
 WHERE a.id=$1::uuid FOR UPDATE OF a`, attemptID).Scan(&a.EnrollmentID, &a.LessonID,
		&a.AttemptNo, &a.RubricVersion, &status, &a.SubmittedAt, &pass, &owner, &entitlementState, &enrollmentState)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.Attempt{}, academy.ErrNotFound
	}
	if err != nil {
		return academy.Attempt{}, err
	}
	if owner != author {
		return academy.Attempt{}, academy.ErrDenied
	}
	if entitlementState != "ACTIVE" {
		return academy.Attempt{}, academy.ErrEntitlement
	}
	if enrollmentState != "ACTIVE" || status != "PENDING" {
		return academy.Attempt{}, academy.ErrState
	}
	result := "FAILED"
	if score >= pass {
		result = "PASSED"
	}
	err = tx.QueryRowContext(ctx, `UPDATE academy_attempts
 SET result=$2,score=$3,evaluated_by=$4::uuid,evaluation_evidence_ref=$5,evaluated_at=now()
 WHERE id=$1::uuid AND result='PENDING' RETURNING evaluated_at`,
		attemptID, result, score, author, evidence).Scan(&a.EvaluatedAt)
	if err != nil {
		return academy.Attempt{}, err
	}
	if result == "PASSED" {
		_, err = tx.ExecContext(ctx, `INSERT INTO academy_progress(enrollment_id,lesson_id,evidence_ref)
  VALUES($1::uuid,$2::uuid,$3) ON CONFLICT DO NOTHING`, a.EnrollmentID, a.LessonID, "academy/attempt/"+attemptID)
		if err != nil {
			return academy.Attempt{}, err
		}
	}
	if err = academyEvent(ctx, tx, author, "academy/enrollment/"+a.EnrollmentID, "academy.assessment.evaluated",
		map[string]any{"attempt_id": attemptID, "lesson_id": a.LessonID, "result": result}); err != nil {
		return academy.Attempt{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Attempt{}, err
	}
	a.ID = attemptID
	a.Result = result
	a.Score = &score
	return a, nil
}

func (c *Checker) AcademyAuthorCourses(actor string) ([]academy.Course, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT c.id::text,c.product_id::text,c.title,c.description,c.state,
 COALESCE(v.id::text,''),COALESCE(v.version_no,0)
 FROM academy_courses c
 LEFT JOIN LATERAL(SELECT id,version_no FROM academy_course_versions
   WHERE course_id=c.id ORDER BY (state='DRAFT') DESC,version_no DESC LIMIT 1)v ON true
 WHERE c.author_id=$1::uuid ORDER BY c.created_at DESC LIMIT 100`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []academy.Course{}
	for rows.Next() {
		var co academy.Course
		if err = rows.Scan(&co.ID, &co.ProductID, &co.Title, &co.Description, &co.State, &co.VersionID, &co.VersionNo); err != nil {
			return nil, err
		}
		result = append(result, co)
	}
	return result, rows.Err()
}
func (c *Checker) AcademyOutline(actor, courseID, versionID string) (map[string]any, error) {
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var author, state string
	err = tx.QueryRowContext(ctx, `SELECT c.author_id::text,v.state FROM academy_course_versions v
 JOIN academy_courses c ON c.id=v.course_id WHERE c.id=$1::uuid AND v.id=$2::uuid`, courseID, versionID).Scan(&author, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, academy.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if actor != author {
		return nil, academy.ErrDenied
	}
	modules := []academy.Module{}
	lessons := []academy.Lesson{}
	rows, err := tx.QueryContext(ctx, `SELECT id::text,version_id::text,ordinal,title FROM academy_modules
 WHERE version_id=$1::uuid ORDER BY ordinal`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m academy.Module
		if err = rows.Scan(&m.ID, &m.VersionID, &m.Ordinal, &m.Title); err != nil {
			rows.Close()
			return nil, err
		}
		modules = append(modules, m)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT l.id::text,l.module_id::text,l.ordinal,l.title,l.kind,l.body,
 COALESCE(l.media_asset_ref,''),COALESCE(a.passing_score,0),COALESCE(a.rubric_version,'')
 FROM academy_modules m JOIN academy_lessons l ON l.module_id=m.id
 LEFT JOIN academy_assessments a ON a.lesson_id=l.id
 WHERE m.version_id=$1::uuid ORDER BY m.ordinal,l.ordinal`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l academy.Lesson
		if err = rows.Scan(&l.ID, &l.ModuleID, &l.Ordinal, &l.Title, &l.Kind, &l.Body, &l.MediaAssetRef, &l.PassingScore, &l.RubricVersion); err != nil {
			rows.Close()
			return nil, err
		}
		lessons = append(lessons, l)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return map[string]any{"state": state, "version_id": versionID, "modules": modules, "lessons": lessons}, nil
}
func (c *Checker) AcademyMyEnrollments(actor string) ([]academy.Enrollment, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT e.id::text,e.version_id::text,e.state,e.enrolled_at,e.completed_at,
 CASE WHEN academy_entitlement_is_active(e.entitlement_id) THEN 'ACTIVE' ELSE 'REVOKED' END,
 c.id::text,COALESCE(v.title_snapshot,c.title),v.version_no
 FROM academy_enrollments e
 JOIN academy_course_versions v ON v.id=e.version_id
 JOIN academy_courses c ON c.id=v.course_id
 WHERE e.identity_id=$1::uuid ORDER BY e.enrolled_at DESC LIMIT 100`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []academy.Enrollment{}
	for rows.Next() {
		var e academy.Enrollment
		if err = rows.Scan(&e.ID, &e.VersionID, &e.State, &e.EnrolledAt, &e.CompletedAt, &e.AccessState, &e.CourseID, &e.CourseTitle, &e.VersionNo); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (c *Checker) AcademyPendingReviews(author string) ([]academy.ReviewAttempt, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT a.id::text,a.enrollment_id::text,a.lesson_id::text,
 a.attempt_no,a.response_text,a.submitted_at,a.rubric_version,
 assessment.passing_score,c.title,l.title
 FROM academy_attempts a
 JOIN academy_assessments assessment ON assessment.lesson_id=a.lesson_id
 JOIN academy_lessons l ON l.id=a.lesson_id
 JOIN academy_modules m ON m.id=l.module_id
 JOIN academy_course_versions v ON v.id=m.version_id
 JOIN academy_courses c ON c.id=v.course_id
 JOIN academy_enrollments e ON e.id=a.enrollment_id
 WHERE c.author_id=$1::uuid AND a.result='PENDING' AND e.state='ACTIVE' AND academy_entitlement_is_active(e.entitlement_id)
 ORDER BY a.submitted_at,a.id LIMIT 100`, author)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pending := []academy.ReviewAttempt{}
	for rows.Next() {
		var item academy.ReviewAttempt
		err = rows.Scan(&item.AttemptID, &item.EnrollmentID, &item.LessonID,
			&item.AttemptNo, &item.ResponseText, &item.SubmittedAt, &item.RubricVersion,
			&item.PassingScore, &item.CourseTitle, &item.LessonTitle)
		if err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	return pending, rows.Err()
}
