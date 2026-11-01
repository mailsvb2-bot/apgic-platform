package runtimepostgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAcademyStandaloneDraftAtomicAndUnpublished(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
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
	actor, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", actor); err != nil {
		t.Fatal(err)
	}
	course, err := store.AcademyCreateStandaloneDraft(actor, "  Мой первый курс  ", "Обучение")
	if err != nil {
		t.Fatal(err)
	}
	if course.State != "DRAFT" || course.Title != "Мой первый курс" || course.VersionNo != 1 {
		t.Fatalf("unexpected draft: %+v", course)
	}
	var status, owner string
	if err = db.QueryRow("SELECT status, owner_id::text FROM products WHERE id=$1::uuid", course.ProductID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "DRAFT" || owner != actor {
		t.Fatalf("unexpected Commerce status=%s owner=%s", status, owner)
	}
	courses, err := store.AcademyAuthorCourses(actor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range courses {
		if item.ID == course.ID && item.VersionID == course.VersionID {
			found = true
		}
	}
	if !found {
		t.Fatalf("standalone course missing from author's persisted course list")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1", "academy/course/"+course.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected single event, got %d err=%v", n, err)
	}
}
