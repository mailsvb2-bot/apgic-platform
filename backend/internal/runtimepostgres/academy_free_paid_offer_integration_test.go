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

// End-to-end DB proof: free admission is a real non-payment entitlement;
// setting a paid price never generates PSP capture or self-attests an order.
func TestAcademyOwnerPriceAndFreeEnrollmentNoPayment(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires the isolated migrated Academy PostgreSQL database")
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	author, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	learner, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{author, learner, outsider} {
		if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	course, err := store.AcademyCreateStandaloneDraft(author, "Авторский бесплатный курс", "Сначала автор создаёт программу")
	if err != nil {
		t.Fatal(err)
	}
	module, err := store.AcademyAddModule(author, course.ID, course.VersionID, academy.Module{Title: "Начало", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID,
		academy.Lesson{Title: "Введение", Kind: academy.Reading, Ordinal: 1, Body: "Авторский текст курса"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyDeclareOriginalRights(author, course.ID, course.VersionID,
		"original/course-created-by-author"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyPublish(author, course.ID, course.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademySetCourseOffer(outsider, course.ID, "FREE", 0); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign offer change allowed: %v", err)
	}
	for _, tc := range []struct {
		mode  string
		price int64
	}{
		{"FREE", 100}, {"PAID", 0}, {"PAID", -1}, {"PAID", 10000000001}, {"OPEN", 0},
	} {
		if _, err = store.AcademySetCourseOffer(author, course.ID, tc.mode, tc.price); !errors.Is(err, academy.ErrInvalid) {
			t.Fatalf("invalid terms accepted %+v: %v", tc, err)
		}
	}
	if _, err = store.AcademyEnroll(learner, course.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("unlisted course admitted a learner: %v", err)
	}
	free, err := store.AcademySetCourseOffer(author, course.ID, "FREE", 0)
	if err != nil || free.Revision != 1 || free.PriceMinor != 0 {
		t.Fatalf("free offer not persisted: %+v %v", free, err)
	}
	replayed, err := store.AcademySetCourseOffer(author, course.ID, "FREE", 0)
	if err != nil || replayed.Revision != free.Revision {
		t.Fatalf("idempotent save created new price revision: %+v %v", replayed, err)
	}
	enrolled, err := store.AcademyEnroll(learner, course.ID)
	if err != nil || enrolled.ID == "" || enrolled.State != "ACTIVE" {
		t.Fatalf("free learner could not enroll: %+v %v", enrolled, err)
	}
	again, err := store.AcademyEnroll(learner, course.ID)
	if err != nil || again.ID != enrolled.ID || again.EntitlementID != enrolled.EntitlementID {
		t.Fatalf("free enrollment replay duplicated the grant: %+v %v", again, err)
	}
	var kind string
	var verification sql.NullString
	if err = db.QueryRow("SELECT kind,verification_id::text FROM store_entitlements WHERE id=$1::uuid",
		enrolled.EntitlementID).Scan(&kind, &verification); err != nil {
		t.Fatal(err)
	}
	if kind != "FREE" || verification.Valid {
		t.Fatalf("free access fabricated a verified payment: kind=%s verification=%+v", kind, verification)
	}
	var captured int
	if err = db.QueryRow(`SELECT count(*) FROM store_transaction_verifications
 WHERE product_ref=$1`, course.ProductID).Scan(&captured); err != nil || captured != 0 {
		t.Fatalf("free course acquired a payment capture: count=%d err=%v", captured, err)
	}
	access, err := store.AcademyMyCourseAccess(learner)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range access {
		if id == course.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("free grant missing from the canonical course access projection")
	}
	paid, err := store.AcademySetCourseOffer(author, course.ID, "PAID", 249900)
	if err != nil || paid.PriceMinor != 249900 || paid.Revision != 2 {
		t.Fatalf("paid price not saved: %+v %v", paid, err)
	}
	if _, err = store.AcademyEnroll(outsider, course.ID); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("paid course admitted unpaid learner: %v", err)
	}
	// Already granted FREE entitlements survive a later tariff change. Only
	// explicit canonical revocation may terminate their immutable history.
	lessons, err := store.AcademyLearning(learner, enrolled.ID)
	if err != nil || len(lessons) != 1 {
		t.Fatalf("prior free learner lost learning after price change: %+v %v", lessons, err)
	}
}
