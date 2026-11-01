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

// Exercises the migrated SQL write guard, not just the HTTP authorization path.
// No cloud credentials, learner playback or payout are enabled by this test.
func TestAcademyCloudMediaDraftOwnerAndVideoBoundaryPostgres(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
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
	outsider, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{author, outsider} {
		if _, err := db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	course, err := store.AcademyCreateStandaloneDraft(author, "Private video authoring", "No public playback")
	if err != nil {
		t.Fatal(err)
	}
	module, err := store.AcademyAddModule(author, course.ID, course.VersionID, academy.Module{Title: "Private module", Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	video, err := store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID,
		academy.Lesson{Title: "Private video", Kind: academy.Video, Ordinal: 1, MediaAssetRef: "private/pending"})
	if err != nil {
		t.Fatal(err)
	}
	reading, err := store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID,
		academy.Lesson{Title: "Reading", Kind: academy.Reading, Ordinal: 2, Body: "Author text"})
	if err != nil {
		t.Fatal(err)
	}
	owned, err := store.AcademyImportCloudMedia(author, []academy.CloudMedia{
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/private/author.mp4", Title: "Private lecture"},
	})
	if err != nil || len(owned) != 1 || owned[0].ID == "" {
		t.Fatalf("author media import: %+v err=%v", owned, err)
	}
	foreign, err := store.AcademyImportCloudMedia(outsider, []academy.CloudMedia{
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/private/foreign.mp4", Title: "Foreign lecture"},
	})
	if err != nil || len(foreign) != 1 {
		t.Fatalf("foreign fixture import: %+v err=%v", foreign, err)
	}
	assignment := []academy.MediaAssignment{{LessonID: video.ID, MediaID: owned[0].ID, Ordinal: 1}}
	if err := store.AcademyAssignCloudMedia(outsider, course.ID, course.VersionID, assignment); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign identity assigned author's media: %v", err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID,
		[]academy.MediaAssignment{{LessonID: video.ID, MediaID: foreign[0].ID, Ordinal: 1}}); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("author assigned foreign cloud file: %v", err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, assignment); err != nil {
		t.Fatalf("draft VIDEO assignment failed: %v", err)
	}
	var firstEvents int
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1 AND event_type='academy.media.assigned'", "academy/version/"+course.VersionID).Scan(&firstEvents); err != nil {
		t.Fatal(err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, assignment); err != nil {
		t.Fatalf("identical assignment retry failed: %v", err)
	}
	var retryEvents int
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1 AND event_type='academy.media.assigned'", "academy/version/"+course.VersionID).Scan(&retryEvents); err != nil {
		t.Fatal(err)
	}
	if retryEvents != firstEvents {
		t.Fatalf("identical retry generated another event: before=%d after=%d", firstEvents, retryEvents)
	}
	view, err := store.AcademyAuthorLessonMedia(author, course.ID, course.VersionID)
	if err != nil || len(view) != 1 || view[0].MediaID != owned[0].ID {
		t.Fatalf("author readback: %+v err=%v", view, err)
	}
	foreignView, err := store.AcademyAuthorLessonMedia(outsider, course.ID, course.VersionID)
	if err != nil || len(foreignView) != 0 {
		t.Fatalf("foreign identity saw private media: %+v err=%v", foreignView, err)
	}
	// The existence of private cloud assignments alone never grants playback
	// or even video metadata to an unenrolled identity.
	if videos, err := store.AcademyLearnerVideoManifest(outsider, course.ID, video.ID); !errors.Is(err, academy.ErrEntitlement) || len(videos) != 0 {
		t.Fatalf("unenrolled user received private video metadata: %+v err=%v", videos, err)
	}
	// Direct SQL cannot bypass the VIDEO-only invariant or owner isolation.
	if _, err := db.Exec("INSERT INTO academy_lesson_media(lesson_id,media_id,ordinal) VALUES($1::uuid,$2::uuid,1)",
		reading.ID, owned[0].ID); err == nil {
		t.Fatal("direct SQL attached cloud media to a READING lesson")
	}
	if _, err := db.Exec("INSERT INTO academy_lesson_media(lesson_id,media_id,ordinal) VALUES($1::uuid,$2::uuid,2)",
		video.ID, foreign[0].ID); err == nil {
		t.Fatal("direct SQL attached another author's media")
	}
	if _, err := db.Exec("UPDATE academy_cloud_media SET source_url='https://example.invalid/new' WHERE id=$1::uuid", owned[0].ID); err == nil {
		t.Fatal("direct SQL rewrote the immutable cloud source")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM academy_lesson_media WHERE lesson_id=$1::uuid", video.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("video assignment count=%d err=%v", count, err)
	}
	if _, err := store.AcademyPublish(author, course.ID, course.VersionID); !errors.Is(err, academy.ErrState) {
		t.Fatalf("VIDEO course became publicly sellable without protected playback: %v", err)
	}
}
