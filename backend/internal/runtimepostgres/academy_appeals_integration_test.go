package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The isolated CI Academy journey creates a real store-verified enrollment
// and human-graded attempt before this test. No synthetic grade or payment
// is introduced by the appeal reviewer route.
func TestAcademyAppealIndependentReviewerAndImmutableEvidence(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated Academy journey database not configured")
	}
	const learner = "00000000-0000-0000-0000-00000000b003"
	const author = "00000000-0000-0000-0000-00000000b001"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var attemptID string
	err = db.QueryRow(`SELECT a.id::text FROM academy_attempts a
 JOIN academy_enrollments e ON e.id=a.enrollment_id
 WHERE e.identity_id=$1::uuid AND a.result='PASSED'
 ORDER BY a.submitted_at DESC LIMIT 1`, learner).Scan(&attemptID)
	if err != nil {
		t.Fatalf("verified paid journey must precede appeal proof: %v", err)
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	reason := "Прошу независимую проверку выставленного результата."
	if _, err = store.AcademySubmitAppeal(author, attemptID, reason); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("author forged learner appeal: %v", err)
	}
	appeal, err := store.AcademySubmitAppeal(learner, attemptID, reason)
	if err != nil || appeal.State != "SUBMITTED" {
		t.Fatalf("graded learner cannot appeal: %+v %v", appeal, err)
	}
	if _, err = store.AcademySubmitAppeal(learner, attemptID, reason); !errors.Is(err, academy.ErrConflict) {
		t.Fatalf("duplicate appeal accepted: %v", err)
	}
	foreign, err := store.AcademyMyAppeals(author)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("author enumerated learner's appeals: %+v %v", foreign, err)
	}
	reviewer, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", reviewer); err != nil {
		t.Fatal(err)
	}
	if err = store.AcademyReviewAppeal(reviewer, appeal.ID, "UNDER_REVIEW", ""); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("unprovisioned reviewer was admitted: %v", err)
	}
	if err = store.AcademyReviewAppeal(author, appeal.ID, "UNDER_REVIEW", ""); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("course author reviewed own appeal: %v", err)
	}
	grantID, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO academy_appeal_reviewer_grants
 (id,reviewer_identity_id,approval_evidence_ref,granted_by)
 VALUES($1::uuid,$2::uuid,'academy/reviewer/approved-by-operator','ci-authorized-operator')`, grantID, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AcademyReviewAppeal(reviewer, appeal.ID, "UNDER_REVIEW", ""); err != nil {
		t.Fatalf("independent reviewer could not accept appeal: %v", err)
	}
	if err = store.AcademyReviewAppeal(reviewer, appeal.ID, "REJECTED", "academy/appeal/reviewed-evidence"); err != nil {
		t.Fatalf("independent reviewer could not resolve appeal: %v", err)
	}
	if err = store.AcademyReviewAppeal(reviewer, appeal.ID, "UPHELD", "academy/appeal/overwrite"); !errors.Is(err, academy.ErrState) {
		t.Fatalf("immutable resolution was overwritten: %v", err)
	}
	if _, err = db.Exec("UPDATE academy_assessment_appeals SET reason='This is a valid length but unauthorized rewrite of the appeal' WHERE id=$1::uuid", appeal.ID); err == nil {
		t.Fatal("direct SQL rewrote learner appeal evidence")
	}
	if _, err = db.Exec("DELETE FROM academy_assessment_appeals WHERE id=$1::uuid", appeal.ID); err == nil {
		t.Fatal("direct SQL deleted immutable appeal")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	appeals, err := store.AcademyMyAppeals(learner)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range appeals {
		if item.ID == appeal.ID {
			found = true
			if item.State != "REJECTED" || item.ResolvedAt == nil || item.Reason != reason {
				t.Fatalf("appeal outcome lost after restart: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("durable independent review missing from learner history")
	}
}
