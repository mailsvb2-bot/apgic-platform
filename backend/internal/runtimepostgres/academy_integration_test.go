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
	// The author picker must use the exact same Commerce ownership rule as
	// AcademyCreateCourse, and must never offer an unpublished draft.
	draftProduct, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO products(
 id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,
 author_refs,name,status
 ) VALUES($1::uuid,'IDENTITY',$2::uuid,$3,$4,ARRAY[$3]::text[],
 'Unpublished course product','DRAFT')`,
		draftProduct, author, "identity/"+author, "identity/"+author); err != nil {
		t.Fatal(err)
	}
	eligible, err := store.AcademyAuthorProducts(author)
	if err != nil {
		t.Fatal(err)
	}
	foundPublished, foundDraft := false, false
	for _, p := range eligible {
		if p.ID == product && p.Name == "Learning contract" {
			foundPublished = true
		}
		if p.ID == draftProduct {
			foundDraft = true
		}
	}
	if !foundPublished || foundDraft {
		t.Fatalf("author Commerce product picker leaked draft or hid published product: %+v", eligible)
	}
	foreignProducts, err := store.AcademyAuthorProducts(foreign)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range foreignProducts {
		if p.ID == product || p.ID == draftProduct {
			t.Fatalf("foreign identity discovered another owner's Commerce product: %+v", p)
		}
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
	// Draft-only scheduling uses the same author and immutable version owner.
	releaseAt := time.Now().UTC().Add(time.Hour)
	if err = store.AcademySetLessonSchedule(foreign, l.ID, &releaseAt, nil); !errors.Is(err, academy.ErrNotFound) {
		t.Fatalf("foreign author changed lesson schedule: %v", err)
	}
	if err = store.AcademySetLessonSchedule(author, l.ID, &releaseAt, nil); err != nil {
		t.Fatalf("draft author could not schedule lesson: %v", err)
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
	if err = store.AcademySetLessonSchedule(author, l.ID, &releaseAt, nil); !errors.Is(err, academy.ErrState) {
		t.Fatalf("published lesson schedule could be changed: %v", err)
	}
	if _, err = db.Exec("UPDATE academy_lesson_schedules SET release_at=now() WHERE lesson_id=$1::uuid", l.ID); err == nil {
		t.Fatal("direct SQL changed published schedule")
	}
	if _, err = db.Exec("UPDATE academy_lessons SET body='tampered' WHERE id=$1::uuid", l.ID); err == nil {
		t.Fatal("direct SQL changed immutable published lesson")
	}
	// Unpaid course must not appear as purchasable access to any identity.
	unpaid, err := store.AcademyMyCourseAccess(foreign)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range unpaid {
		if id == created.ID {
			t.Fatal("unverified course appeared as purchased access")
		}
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
	// The learner-facing catalog projection must agree with the exact
	// canonical verification predicate used by the enrollment writer.
	verifiedCourses, err := store.AcademyMyCourseAccess(learner)
	if err != nil {
		t.Fatal(err)
	}
	foundPaidCourse := false
	for _, id := range verifiedCourses {
		if id == course.ID {
			foundPaidCourse = true
		}
	}
	if !foundPaidCourse {
		t.Fatal("verified captured course missing from learner purchase readback")
	}
	foreignAccess, err := store.AcademyMyCourseAccess(author)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range foreignAccess {
		if id == course.ID {
			t.Fatal("unpaid author saw another identity's purchased course")
		}
	}
	enrollment, err := store.AcademyEnroll(learner, course.ID)
	if err != nil {
		t.Fatalf("store-verified purchase did not unlock enrollment: %v", err)
	}
	if enrollment.EntitlementID != paidEntitlement || enrollment.State != "ACTIVE" {
		t.Fatalf("course granted access without bound verified purchase: %+v", enrollment)
	}
	// Replay must preserve the exact historical purchase basis and enrollment ID.
	// A second enrollment may not silently attach a different payment.
	replayedEnrollment, err := store.AcademyEnroll(learner, course.ID)
	if err != nil || replayedEnrollment.ID != enrollment.ID ||
		replayedEnrollment.EntitlementID != paidEntitlement || replayedEnrollment.AccessState != "ACTIVE" {
		t.Fatalf("verified enrollment replay changed immutable payment basis: %+v %v", replayedEnrollment, err)
	}
	// A newer independent purchase must not make a terminal enrollment
	// look actionable in the learner catalog. Probe the exact shared SQL
	// inside a rollback-only transaction: immutable production history stays
	// untouched for the remainder of the complete paid learning journey.
	probeTx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer probeTx.Rollback()
	var canonicalCaptureActive bool
	if err = probeTx.QueryRowContext(ctx,
		"SELECT academy_entitlement_is_active($1::uuid)", paidEntitlement).Scan(&canonicalCaptureActive); err != nil || !canonicalCaptureActive {
		t.Fatalf("verified captured purchase not accepted by shared Academy gate: active=%v err=%v", canonicalCaptureActive, err)
	}
	if _, err = probeTx.ExecContext(ctx,
		"UPDATE academy_enrollments SET state='REVOKED' WHERE id=$1::uuid", enrollment.ID); err != nil {
		t.Fatalf("terminal enrollment probe failed: %v", err)
	}
	var incorrectlyOffered bool
	if err = probeTx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 `+academyVerifiedEnrollmentFrom+`
		 AND c.id=$2::uuid)`, learner, course.ID).Scan(&incorrectlyOffered); err != nil {
		t.Fatal(err)
	}
	if incorrectlyOffered {
		t.Fatal("terminal historical enrollment was offered again despite immutable purchase binding")
	}
	if err = probeTx.Rollback(); err != nil {
		t.Fatal(err)
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
	available, err := store.AcademyLearnerCohorts(learner, enrollment.ID)
	if err != nil || len(available) != 1 || available[0].ID != cohort.ID || available[0].IsMember {
		t.Fatalf("learner cohort discovery did not expose only the paid version before joining: %+v %v", available, err)
	}
	if _, err = store.AcademyLearnerCohorts(author, enrollment.ID); !errors.Is(err, academy.ErrNotFound) {
		t.Fatalf("foreign identity enumerated paid learner's cohorts: %v", err)
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
	joined, err := store.AcademyLearnerCohorts(learner, enrollment.ID)
	if err != nil || len(joined) != 1 || !joined[0].IsMember || joined[0].Members != 1 {
		t.Fatalf("cohort membership did not persist into learner readback: %+v %v", joined, err)
	}
	var members int
	if err = store.db.QueryRow(`SELECT count(*) FROM academy_cohort_memberships WHERE cohort_id=$1::uuid`, cohort.ID).Scan(&members); err != nil || members != 1 {
		t.Fatalf("cohort replay created duplicate membership: count=%d err=%v", members, err)
	}
	otherCohort, err := store.AcademyCreateCohort(author, course.VersionID, "Parallel cohort", 2, &starts, &closes)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AcademyJoinCohort(learner, otherCohort.ID, enrollment.ID); !errors.Is(err, academy.ErrConflict) {
		t.Fatalf("enrollment admitted to two parallel cohorts: %v", err)
	}
	if _, err = store.db.Exec(`INSERT INTO academy_cohort_memberships(cohort_id,enrollment_id)
 VALUES($1::uuid,$2::uuid)`, otherCohort.ID, enrollment.ID); err == nil {
		t.Fatal("direct SQL bypassed one-cohort-per-enrollment invariant")
	}
	if _, err = store.db.Exec(`DELETE FROM academy_cohort_memberships
 WHERE cohort_id=$1::uuid AND enrollment_id=$2::uuid`, cohort.ID, enrollment.ID); err == nil {
		t.Fatal("direct SQL deleted immutable cohort admission evidence")
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

	// The learner sees their own pending work but a foreign identity cannot
	// enumerate grading outcomes. No instructor evidence is projected.
	if _, err = store.AcademyLearnerAttempts(author, enrollment.ID); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign identity read learner assessment history: %v", err)
	}
	pendingHistory, err := store.AcademyLearnerAttempts(learner, enrollment.ID)
	if err != nil || len(pendingHistory) != 1 || pendingHistory[0].ID != attempt.ID ||
		pendingHistory[0].Result != "PENDING" || pendingHistory[0].Score != nil {
		t.Fatalf("pending attempt history is incorrect: %+v %v", pendingHistory, err)
	}
	// Author reporting must distinguish immutable historical enrollment from
	// actionable access and review work. A terminal enrollment with an
	// otherwise still captured entitlement cannot be counted as active or
	// eligible for grading. Exercise the production query under rollback so
	// the learner can continue the real paid journey afterward.
	beforeReport, err := store.AcademyAuthorReport(author)
	if err != nil {
		t.Fatal(err)
	}
	foundActionable := false
	for _, item := range beforeReport.Courses {
		if item.CourseID == course.ID {
			foundActionable = true
			if item.ActiveAccess != 1 || item.PendingAssessments != 1 {
				t.Fatalf("paid learner's active access and pending review not reported: %+v", item)
			}
		}
	}
	if !foundActionable {
		t.Fatal("published paid course missing from author report")
	}
	metricsProbeTx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer metricsProbeTx.Rollback()
	if _, err = metricsProbeTx.ExecContext(ctx,
		"UPDATE academy_enrollments SET state='REVOKED' WHERE id=$1::uuid", enrollment.ID); err != nil {
		t.Fatalf("cannot probe terminal enrollment report: %v", err)
	}
	metricsRows, err := metricsProbeTx.QueryContext(ctx, academyAuthorCourseMetricsQuery, author)
	if err != nil {
		t.Fatal(err)
	}
	foundHistorical := false
	for metricsRows.Next() {
		var id, title string
		var total, completed, active, pending, passed int
		if err = metricsRows.Scan(&id, &title, &total, &completed, &active, &pending, &passed); err != nil {
			metricsRows.Close()
			t.Fatal(err)
		}
		if id == course.ID {
			foundHistorical = true
			if total != 1 || completed != 0 || active != 0 || pending != 0 || passed != 0 {
				metricsRows.Close()
				t.Fatalf("terminal enrollment inflated actionable author metrics: total=%d completed=%d active=%d pending=%d passed=%d", total, completed, active, pending, passed)
			}
		}
	}
	if err = metricsRows.Err(); err != nil {
		metricsRows.Close()
		t.Fatal(err)
	}
	metricsRows.Close()
	if !foundHistorical {
		t.Fatal("terminal historical enrollment disappeared from author reporting")
	}
	if err = metricsProbeTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	afterReport, err := store.AcademyAuthorReport(author)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range afterReport.Courses {
		if item.CourseID == course.ID && (item.ActiveAccess != 1 || item.PendingAssessments != 1) {
			t.Fatalf("rollback did not restore actionable author reporting: %+v", item)
		}
	}
	if _, err = store.AcademySubmitAttempt(learner, enrollment.ID, assessment.ID, "duplicate before review"); !errors.Is(err, academy.ErrState) {
		t.Fatalf("pending review accepted duplicate submission: %v", err)
	}
	if _, err = store.AcademyEvaluateAttempt(learner, attempt.ID, "review/unauthorized", 95); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("learner graded their own submission: %v", err)
	}
	evaluated, err := store.AcademyEvaluateAttempt(author, attempt.ID, "academy/human-review/ci", 95)
	if err != nil || evaluated.Result != "PASSED" {
		t.Fatalf("instructor review did not persist: %+v: %v", evaluated, err)
	}

	passedHistory, err := store.AcademyLearnerAttempts(learner, enrollment.ID)
	if err != nil || len(passedHistory) != 1 || passedHistory[0].Result != "PASSED" ||
		passedHistory[0].Score == nil || *passedHistory[0].Score != 95 || passedHistory[0].EvaluatedAt == nil {
		t.Fatalf("evaluated result not visible to owner: %+v %v", passedHistory, err)
	}
	if _, err = store.AcademySubmitAttempt(learner, enrollment.ID, assessment.ID, "duplicate after pass"); !errors.Is(err, academy.ErrState) {
		t.Fatalf("passed assessment accepted duplicate submission: %v", err)
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

	restartedHistory, err := store.AcademyLearnerAttempts(learner, enrollment.ID)
	if err != nil || len(restartedHistory) != 1 || restartedHistory[0].Result != "PASSED" {
		t.Fatalf("restart lost learner grading evidence: %+v %v", restartedHistory, err)
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
	// The certificate remains discoverable after a process restart without
	// calling the completion mutation again. Another identity sees no token.
	ownedCertificates, err := store.AcademyMyCertificates(learner)
	if err != nil || len(ownedCertificates) != 1 ||
		ownedCertificates[0].ID != certificate.ID ||
		ownedCertificates[0].State != "ACTIVE" ||
		ownedCertificates[0].LearningAccessState != "ACTIVE" ||
		ownedCertificates[0].VerificationURL != certificate.VerificationURL ||
		ownedCertificates[0].EvidenceCandidateState != academy.EvidenceCandidateStatus {
		t.Fatalf("durable learner certificate readback failed: %+v %v", ownedCertificates, err)
	}
	foreignCertificates, err := store.AcademyMyCertificates(author)
	if err != nil || len(foreignCertificates) != 0 {
		t.Fatalf("foreign identity read learner certificate: %+v %v", foreignCertificates, err)
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
	if _, err = store.AcademyLearnerCohorts(learner, enrollmentID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("revoked entitlement still disclosed learner cohort schedule: %v", err)
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
	// Refund must not hide the earned credential or falsely mark the old
	// entitlement active. Certificate state is independent of learning access.
	refundedCertificates, err := store.AcademyMyCertificates(learner)
	if err != nil || len(refundedCertificates) != 1 ||
		refundedCertificates[0].EnrollmentID != enrollmentID ||
		refundedCertificates[0].State != "ACTIVE" ||
		refundedCertificates[0].LearningAccessState != "REVOKED" {
		t.Fatalf("refund erased or misrepresented historical credential: %+v %v", refundedCertificates, err)
	}

	var courseID string
	if err = db.QueryRow(`SELECT v.course_id::text FROM academy_enrollments e
 JOIN academy_course_versions v ON v.id=e.version_id WHERE e.id=$1::uuid`, enrollmentID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	// A canonical refund must also remove actionable author access, while
	// retaining the immutable course completion and historical gross capture.
	// Gross captured volume is not net author revenue after refund.
	const author = "00000000-0000-0000-0000-00000000b001"
	authorReport, err := store.AcademyAuthorReport(author)
	if err != nil {
		t.Fatalf("author report after canonical refund: %v", err)
	}
	if authorReport.RevenueState != academy.RevenueGrossOnly {
		t.Fatalf("refund was misrepresented as reconciled author revenue: %q", authorReport.RevenueState)
	}
	foundHistoricalCourse := false
	for _, row := range authorReport.Courses {
		if row.CourseID != courseID {
			continue
		}
		foundHistoricalCourse = true
		if row.Enrollments != 1 || row.Completed != 1 || row.ActiveAccess != 0 ||
			row.PendingAssessments != 0 || row.PassedAssessments != 1 {
			t.Fatalf("refunded enrollment inflated actionable author metrics or lost history: %+v", row)
		}
	}
	if !foundHistoricalCourse {
		t.Fatal("refunded completed course disappeared from author history")
	}
	foundHistoricalCapture := false
	for _, capture := range authorReport.GrossCaptures {
		if capture.CourseID != courseID {
			continue
		}
		foundHistoricalCapture = true
		if capture.Currency != "RUB" || capture.CapturedCount != 1 || capture.CapturedGrossMinor != 10000 {
			t.Fatalf("refund rewrote immutable gross capture evidence: %+v", capture)
		}
	}
	if !foundHistoricalCapture {
		t.Fatal("refunded course lost original canonical capture evidence")
	}
	// A refund must immediately remove the course from the read-only
	// learner catalog projection, without erasing historical completion.
	stillPurchased, err := store.AcademyMyCourseAccess(learner)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range stillPurchased {
		if id == courseID {
			t.Fatal("refunded course still appears as verified learner access")
		}
	}
	if _, err = store.AcademyEnroll(learner, courseID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("revoked purchase reopened course enrollment: %v", err)
	}
}
