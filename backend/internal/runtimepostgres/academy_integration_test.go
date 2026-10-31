package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// TestAcademyVersioningRequiresVerifiedAccess proves the author -> curriculum ->
// publication boundary against actual PostgreSQL and refuses synthetic enrollment.
// Run against the isolated CI migration database, never a production database.
func TestAcademyVersioningRequiresVerifiedAccess(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated Academy database not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	author, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	product, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{author, foreign} {
		if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO products(
 id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,
 author_refs,name,status,published_at
 )VALUES($1::uuid,'IDENTITY',$2::uuid,$3,$4,ARRAY[$3]::text[],
 'Learning contract','PUBLISHED',now())`,
		product, author, "identity/"+author, "identity/"+author)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyCreateCourse(foreign, academy.Course{ProductID: product, Title: "Foreign course"}); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign author must not publish other user's product: %v", err)
	}
	created, err := store.AcademyCreateCourse(author, academy.Course{
		ProductID: product, Title: "Canonical learning", Description: "Structured course"})
	if err != nil {
		t.Fatal(err)
	}
	if created.VersionNo != 1 || created.State != "DRAFT" {
		t.Fatalf("bad first version: %+v", created)
	}
	if _, err = store.AcademyNewVersion(author, created.ID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("parallel draft version permitted: %v", err)
	}
	if _, err = store.AcademyPublish(author, created.ID, created.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("empty version was published: %v", err)
	}
	m, err := store.AcademyAddModule(author, created.ID, created.VersionID,
		academy.Module{Title: "Introduction", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	l, err := store.AcademyAddLesson(author, created.ID, created.VersionID, m.ID,
		academy.Lesson{Title: "Start here", Kind: academy.Reading, Ordinal: 1, Body: "Learning evidence matters."})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AcademyAddLesson(foreign, created.ID, created.VersionID, m.ID,
		academy.Lesson{Title: "Hijack", Kind: academy.Reading, Ordinal: 2, Body: "attack"})
	if !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign author modified draft: %v", err)
	}
	if _, err = store.AcademyPublish(author, created.ID, created.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("published without author ownership declaration: %v", err)
	}
	if _, err = store.AcademyDeclareOriginalRights(foreign, created.ID, created.VersionID, "original/unauthorized-author"); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign author attested a course: %v", err)
	}
	original, err := store.AcademyDeclareOriginalRights(author, created.ID, created.VersionID, "original/training-materials-created-by-author")
	if err != nil || len(original.CurriculumSHA256) != 64 {
		t.Fatalf("failed to attest original curriculum: %+v %v", original, err)
	}
	replayed, err := store.AcademyDeclareOriginalRights(author, created.ID, created.VersionID, "original/another-replayed-source")
	if err != nil || replayed.ID != original.ID || replayed.ProvenanceRef != original.ProvenanceRef {
		t.Fatalf("replay changed immutable original rights evidence: old=%+v replay=%+v err=%v", original, replayed, err)
	}
	var declarations int
	if err = db.QueryRow("SELECT count(*) FROM academy_original_rights WHERE version_id=$1::uuid", created.VersionID).Scan(&declarations); err != nil || declarations != 1 {
		t.Fatalf("duplicate author declaration was persisted: count=%d err=%v", declarations, err)
	}
	// Privileged direct SQL still cannot fabricate authorship or rewrite the
	// immutable provenance after the original learner's course is published.
	forgedID, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO academy_original_rights
 (id,version_id,author_id,rights_basis,provenance_ref,policy_version,curriculum_sha256)
 VALUES($1::uuid,$2::uuid,$3::uuid,'ORIGINAL_AUTHOR_DECLARATION',
 'forged/third-party-copyright','academy-original-rights-v1',$4)`,
		forgedID, created.VersionID, foreign, original.CurriculumSHA256); err == nil {
		t.Fatal("direct SQL forged foreign author's rights to copyrighted curriculum")
	}
	if _, err = db.Exec("UPDATE academy_original_rights SET provenance_ref='forged/rewrite' WHERE id=$1::uuid", original.ID); err == nil {
		t.Fatal("original author attestation was rewritten")
	}
	if _, err = db.Exec("DELETE FROM academy_original_rights WHERE id=$1::uuid", original.ID); err == nil {
		t.Fatal("original author attestation was deleted")
	}
	if _, err = db.Exec("UPDATE academy_modules SET title='Changed after declaration' WHERE id=$1::uuid", m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyPublish(author, created.ID, created.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("publication ignored changed curriculum after author declaration: %v", err)
	}
	if _, err = store.AcademyDeclareOriginalRights(author, created.ID, created.VersionID, "original/updated-materials-created-by-author"); err != nil {
		t.Fatalf("updated curriculum declaration failed: %v", err)
	}
	published, err := store.AcademyPublish(author, created.ID, created.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "PUBLISHED" {
		t.Fatalf("missing publication: %+v", published)
	}
	if _, err = store.AcademyDeclareOriginalRights(author, created.ID, created.VersionID, "original/post-publication-retcon"); !errors.Is(err, academy.ErrState) {
		t.Fatalf("accepted a new rights declaration after immutable publication: %v", err)
	}
	_, err = store.AcademyAddLesson(author, created.ID, created.VersionID, m.ID,
		academy.Lesson{Title: "Retcon", Kind: academy.Reading, Ordinal: 2, Body: "rewrite"})
	if !errors.Is(err, academy.ErrState) {
		t.Fatalf("published lesson changed: %v", err)
	}
	if _, err = db.Exec("UPDATE academy_lessons SET body='tampered' WHERE id=$1::uuid", l.ID); err == nil {
		t.Fatal("direct SQL changed immutable published lesson")
	}
	if _, err = store.AcademyEnroll(author, created.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("enrollment without provider verified entitlement accepted: %v", err)
	}
	if _, err = store.AcademyEnroll(foreign, created.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("foreign enrollment without payment accepted: %v", err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM academy_enrollments WHERE version_id=$1::uuid", created.VersionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unverified payment created %d enrollments", count)
	}
	second, err := store.AcademyNewVersion(author, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.VersionNo != 2 || second.VersionID == created.VersionID {
		t.Fatalf("version did not advance: %+v", second)
	}

	// SQL privilege-level regression: neither UPDATE/DELETE nor moving records
	// into a fresh draft may rewrite the original published learning evidence.
	for _, mutation := range []struct {
		label, statement string
		args             []any
	}{
		{"delete published module", "DELETE FROM academy_modules WHERE id=$1::uuid", []any{m.ID}},
		{"delete published lesson", "DELETE FROM academy_lessons WHERE id=$1::uuid", []any{l.ID}},
		{"move published module to draft", "UPDATE academy_modules SET version_id=$2::uuid WHERE id=$1::uuid", []any{m.ID, second.VersionID}},
		{"retitle published snapshot", "UPDATE academy_course_versions SET title_snapshot='Tampered Certificate' WHERE id=$1::uuid", []any{created.VersionID}},
		{"rewrite published rubric", "UPDATE academy_course_versions SET completion_policy_version='forged-v9' WHERE id=$1::uuid", []any{created.VersionID}},
	} {
		if _, err = db.Exec(mutation.statement, mutation.args...); err == nil {
			t.Fatalf("direct SQL illegally modified published evidence: %s", mutation.label)
		}
	}
	// The credential verifier must not depend on mutable marketing copy.
	var historicalTitle string
	if err = db.QueryRow("SELECT title_snapshot FROM academy_course_versions WHERE id=$1::uuid", created.VersionID).Scan(&historicalTitle); err != nil {
		t.Fatal(err)
	}
	if historicalTitle != "Canonical learning" {
		t.Fatalf("published title snapshot changed: %s", historicalTitle)
	}
	if _, err = db.Exec("UPDATE academy_courses SET title='Marketing title' WHERE id=$1::uuid", created.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT title_snapshot FROM academy_course_versions WHERE id=$1::uuid", created.VersionID).Scan(&historicalTitle); err != nil {
		t.Fatal(err)
	}
	if historicalTitle != "Canonical learning" {
		t.Fatalf("updated course marketing title rewrote original version: %s", historicalTitle)
	}

	var oldText string
	if err = db.QueryRow("SELECT body FROM academy_lessons WHERE id=$1::uuid", l.ID).Scan(&oldText); err != nil {
		t.Fatal(err)
	}
	if oldText != "Learning evidence matters." {
		t.Fatalf("completed lesson history changed: %q", oldText)
	}
	var outboxCount int
	if err = db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref='academy/course/'||$1", created.ID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount < 3 {
		t.Fatalf("durable author events missing: %d", outboxCount)
	}
	var brokenOrdering int
	if err = db.QueryRow(`SELECT count(*) FROM (
  SELECT aggregate_ref,aggregate_version,count(*) AS duplicates
  FROM outbox_events WHERE producer='apgic-academy'
  GROUP BY aggregate_ref,aggregate_version
  HAVING aggregate_version IS NULL OR aggregate_version < 1 OR count(*) > 1
 ) invalid_events`).Scan(&brokenOrdering); err != nil {
		t.Fatal(err)
	}
	if brokenOrdering != 0 {
		t.Fatalf("Academy emitted %d unordered or duplicate-version durable event groups", brokenOrdering)
	}
}

// TestAcademyStoreVerifiedPaidJourneySurvivesRestart exercises the full
// persisted learner journey against a store transaction that was verified by
// the existing canonical PAYMENT_EFFECT -> LEDGER -> STORE_VERIFICATION gates.
// The preceding ci_r2_mobile_store_invariants.sql supplies a synthetic,
// provider-verified CI transaction. It is not evidence of a live payment.
func TestAcademyStoreVerifiedPaidJourneySurvivesRestart(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL Academy journey database not configured")
	}
	const author = "00000000-0000-0000-0000-00000000b001"
	const learner = "00000000-0000-0000-0000-00000000b003"
	const product = "00000000-0000-0000-0000-00000000af03"
	const paidEntitlement = "00000000-0000-0000-0000-00000000a902"
	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	course, err := store.AcademyCreateCourse(author, academy.Course{ProductID: product, Title: "Verified paid course", Description: "Commercial learning journey"})
	if err != nil {
		t.Fatalf("author cannot create the store-bound course: %v", err)
	}
	module, err := store.AcademyAddModule(author, course.ID, course.VersionID, academy.Module{Title: "Module A", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	reading, err := store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID, academy.Lesson{
		Title: "Persisting progress", Kind: academy.Reading, Ordinal: 1, Body: "Knowledge persists after a restart.",
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID, academy.Lesson{
		Title: "Checked understanding", Kind: academy.Assessment, Ordinal: 2, PassingScore: 70, RubricVersion: "rubric-pay-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyPublish(author, course.ID, course.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("paid course published without author rights attestation: %v", err)
	}
	if _, err = store.AcademyDeclareOriginalRights(author, course.ID, course.VersionID, "original/verified-paid-curriculum-authored-content"); err != nil {
		t.Fatalf("failed to attest paid curriculum: %v", err)
	}
	published, err := store.AcademyPublish(author, course.ID, course.VersionID)
	if err != nil || published.State != "PUBLISHED" {
		t.Fatalf("published course missing: %+v: %v", published, err)
	}
	if _, err = store.AcademyEnroll(author, course.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("author without purchased entitlement enrolled: %v", err)
	}
	enrollment, err := store.AcademyEnroll(learner, course.ID)
	if err != nil {
		t.Fatalf("store-verified purchase did not unlock enrollment: %v", err)
	}
	if enrollment.EntitlementID != paidEntitlement || enrollment.State != "ACTIVE" {
		t.Fatalf("course granted access without bound verified purchase: %+v", enrollment)
	}
	// A real store-verified enrollment can join a scheduled cohort, but only
	// through the canonical Academy writer and within the purchased version.
	starts := time.Now().UTC().Add(4 * time.Hour)
	closes := starts.Add(-time.Hour)
	cohort, err := store.AcademyCreateCohort(author, course.VersionID, "October cohort", 1, &starts, &closes)
	if err != nil {
		t.Fatalf("author cannot create scheduled cohort: %v", err)
	}
	if _, err = store.AcademyCreateCohort(learner, course.VersionID, "Foreign cohort", 1, &starts, &closes); !errors.Is(err, academy.ErrNotFound) {
		t.Fatalf("learner created another author's cohort: %v", err)
	}
	cohorts, err := store.AcademyListCohorts(author)
	if err != nil || len(cohorts) == 0 {
		t.Fatalf("author cannot see cohort: %+v %v", cohorts, err)
	}
	if err = store.AcademyJoinCohort(author, cohort.ID, enrollment.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("foreign identity joined paid cohort: %v", err)
	}
	if err = store.AcademyJoinCohort(learner, cohort.ID, enrollment.ID); err != nil {
		t.Fatalf("paid learner cannot join cohort: %v", err)
	}
	if err = store.AcademyJoinCohort(learner, cohort.ID, enrollment.ID); err != nil {
		t.Fatalf("idempotent cohort replay failed: %v", err)
	}
	learnerHistory, err := store.AcademyMyEnrollments(learner)
	if err != nil {
		t.Fatalf("learner history unavailable: %v", err)
	}
	seenCourse := false
	for _, item := range learnerHistory {
		if item.ID == enrollment.ID {
			seenCourse = true
			if item.CourseID != course.ID || item.CourseTitle != "Verified paid course" || item.VersionNo != 1 {
				t.Fatalf("learner history lost the immutable published course identity: %+v", item)
			}
		}
	}
	if !seenCourse {
		t.Fatal("paid course missing from learner history")
	}
	foreignHistory, err := store.AcademyMyEnrollments(author)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range foreignHistory {
		if item.ID == enrollment.ID {
			t.Fatalf("learner history leaked to unrelated identity: %+v", item)
		}
	}
	if _, err = store.AcademyLearning(author, enrollment.ID); err == nil {
		t.Fatal("non-learner read purchased content")
	}
	if err = store.AcademyMarkProgress(learner, enrollment.ID, assessment.ID); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("assessment bypass without review: %v", err)
	}
	if _, err = store.AcademyComplete(learner, enrollment.ID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("early completion bypassed lesson evidence: %v", err)
	}
	if err = store.AcademyMarkProgress(learner, enrollment.ID, reading.ID); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.AcademySubmitAttempt(learner, enrollment.ID, assessment.ID, "Course progress is transactional PostgreSQL state.")
	if err != nil {
		t.Fatalf("student assessment not submitted: %v", err)
	}
	if _, err = store.AcademyEvaluateAttempt(learner, attempt.ID, "review/unauthorized", 95); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("learner graded their own submission: %v", err)
	}
	evaluated, err := store.AcademyEvaluateAttempt(author, attempt.ID, "academy/human-review/ci", 95)
	if err != nil || evaluated.Result != "PASSED" {
		t.Fatalf("instructor review did not persist: %+v: %v", evaluated, err)
	}
	// Drop the runtime before completing the course: progress must NOT depend
	// on any process-local cache or React Native/Web application memory.
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	lessons, err := store.AcademyLearning(learner, enrollment.ID)
	if err != nil {
		t.Fatalf("learner cannot resume after process restart: %v", err)
	}
	if len(lessons) != 2 || !lessons[0].Completed || !lessons[1].Completed {
		t.Fatalf("restart lost paid progress: %+v", lessons)
	}
	certificate, err := store.AcademyComplete(learner, enrollment.ID)
	if err != nil {
		t.Fatalf("completed paid course did not issue certificate: %v", err)
	}
	if certificate.State != "ACTIVE" || certificate.EvidenceCandidateState != academy.EvidenceCandidateStatus || len(certificate.VerificationToken) < 32 {
		t.Fatalf("unsafe/invalid certificate outcome: %+v", certificate)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	certificateAgain, err := store.AcademyComplete(learner, enrollment.ID)
	if err != nil || certificateAgain.ID != certificate.ID {
		t.Fatalf("certificate is not idempotent after restart: %+v %v", certificateAgain, err)
	}
	verified, err := store.AcademyVerify(certificate.VerificationToken)
	if err != nil || verified["state"] != "ACTIVE" || verified["qualification_status"] != "NOT_GRANTED" {
		t.Fatalf("public verification does not match durable certificate: %+v %v", verified, err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var verification string
	err = db.QueryRow(`SELECT sv.provider_evidence_ref FROM academy_enrollments e
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN store_transaction_verifications sv ON sv.id=t.verification_id
 WHERE e.id=$1::uuid`, enrollment.ID).Scan(&verification)
	if err != nil || verification == "" {
		t.Fatalf("purchase missing persisted provider verification evidence: %v", err)
	}
	var count int
	err = db.QueryRow("SELECT count(*) FROM academy_progress WHERE enrollment_id=$1::uuid", enrollment.ID).Scan(&count)
	if err != nil || count != 2 {
		t.Fatalf("missing durable lesson evidence, count=%d: %v", count, err)
	}
	report, err := store.AcademyAuthorReport(author)
	if err != nil {
		t.Fatalf("author report after restart: %v", err)
	}
	foundCourse := false
	for _, row := range report.Courses {
		if row.CourseID == course.ID {
			foundCourse = true
			if row.Enrollments != 1 || row.Completed != 1 || row.ActiveAccess != 1 || row.PassedAssessments != 1 {
				t.Fatalf("incorrect persisted author metrics: %+v", row)
			}
		}
	}
	if !foundCourse || report.RevenueState != academy.RevenueGrossOnly {
		t.Fatalf("author report missing course or unsafe revenue claim: %+v", report)
	}
	foundCapture := false
	for _, capture := range report.GrossCaptures {
		if capture.CourseID == course.ID {
			foundCapture = true
			if capture.Currency != "RUB" || capture.CapturedCount != 1 || capture.CapturedGrossMinor != 10000 {
				t.Fatalf("captured amount must match canonical order evidence: %+v", capture)
			}
		}
	}
	if !foundCapture {
		t.Fatal("canonical captured order was not reflected in author report")
	}
	foreignReport, err := store.AcademyAuthorReport(learner)
	if err != nil || len(foreignReport.Courses) != 0 || len(foreignReport.GrossCaptures) != 0 {
		t.Fatalf("learner accessed foreign author's report: %+v %v", foreignReport, err)
	}
}

// TestAcademyStoreRefundWithdrawsLearningAccess proves that an external store
// refund revokes learner CONTENT access after previously persisted course
// completion. The original certificate remains separate historical evidence.
func TestAcademyStoreRefundWithdrawsLearningAccess(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated Academy refund database not configured")
	}
	const learner = "00000000-0000-0000-0000-00000000b003"
	const product = "00000000-0000-0000-0000-00000000af03"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var enrollmentID, entitlementState string
	var projectedState, effectiveState string
	err = db.QueryRow(`SELECT e.id::text,t.state,p.entitlement_state,
 CASE WHEN academy_entitlement_is_active(t.id) THEN 'ACTIVE' ELSE 'REVOKED' END
 FROM academy_enrollments e
 JOIN academy_course_versions v ON v.id=e.version_id
 JOIN academy_courses c ON c.id=v.course_id
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN store_subscription_projections p ON p.entitlement_id=t.id
 WHERE e.identity_id=$1::uuid AND c.product_id=$2::uuid`, learner, product).
		Scan(&enrollmentID, &entitlementState, &projectedState, &effectiveState)
	if err != nil {
		t.Fatalf("previously completed paid journey must remain durable after refund: %v", err)
	}
	if entitlementState != "ACTIVE" || projectedState != "REVOKED" || effectiveState != "REVOKED" {
		t.Fatalf("refund must override the original ACTIVE payment: base=%s projection=%s effective=%s",
			entitlementState, projectedState, effectiveState)
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.AcademyLearning(learner, enrollmentID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("revoked purchased lesson body remains accessible: %v", err)
	}
	history, err := store.AcademyMyEnrollments(learner)
	if err != nil {
		t.Fatal(err)
	}
	seenRevoked := false
	for _, entry := range history {
		if entry.ID == enrollmentID {
			seenRevoked = true
			if entry.AccessState != "REVOKED" || entry.State != "COMPLETED" {
				t.Fatalf("refund did not preserve completion history and revoke access: %+v", entry)
			}
		}
	}
	if !seenRevoked {
		t.Fatal("refunded course disappeared from learner's historical enrollment list")
	}

	var courseID string
	if err = db.QueryRow(`SELECT v.course_id::text FROM academy_enrollments e
 JOIN academy_course_versions v ON v.id=e.version_id WHERE e.id=$1::uuid`, enrollmentID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyEnroll(learner, courseID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("revoked purchase reopened course enrollment: %v", err)
	}
}
