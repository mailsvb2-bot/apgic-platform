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
	published, err := store.AcademyPublish(author, created.ID, created.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "PUBLISHED" {
		t.Fatalf("missing publication: %+v", published)
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
}
