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
	// Author curriculum publication must not require a paid product, an
	// enrolled learner, a certified video distribution or a fake checkout.
	module, err := store.AcademyAddModule(actor, course.ID, course.VersionID, academy.Module{
		Title: "Модуль автора", Ordinal: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyAddLesson(actor, course.ID, course.VersionID, module.ID,
		academy.Lesson{Title: "Видео автора", Kind: academy.Video, Ordinal: 1,
			MediaAssetRef: "private/not-delivery-approved"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcademyPublish(actor, course.ID, course.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("unattested curriculum published: %v", err)
	}
	if _, err = store.AcademyDeclareOriginalRights(actor, course.ID, course.VersionID,
		"original/author-produced-course"); err != nil { t.Fatal(err) }
	t.Setenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED", "false")
	published, err := store.AcademyPublish(actor, course.ID, course.VersionID)
	if err != nil || published.State != "PUBLISHED" {
		t.Fatalf("author could not publish without sales or students: %+v %v", published, err)
	}
	if err = db.QueryRow("SELECT status FROM products WHERE id=$1::uuid", course.ProductID).Scan(&status); err != nil { t.Fatal(err) }
	if status != "DRAFT" {
		t.Fatalf("publishing curriculum changed Commerce status: %s", status)
	}
	if err = db.QueryRow("SELECT count(*) FROM academy_enrollments WHERE version_id=$1::uuid", course.VersionID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("publishing invented learner enrollment: %d %v", n, err)
	}
	if access, err := store.AcademyMyCourseAccess(actor); err != nil {
		t.Fatal(err)
	} else {
		for _, id := range access {
			if id == course.ID {
				t.Fatal("published course granted unauthorized learner video access")
			}
		}
	}
}
