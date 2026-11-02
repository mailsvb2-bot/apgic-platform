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
		// Pasted duplicates must not trigger a second update or return a
		// duplicate ID in the bulk import response.
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/private/author.mp4", Title: "Duplicate pasted row"},
	})
	if err != nil || len(owned) != 1 || owned[0].ID == "" {
		t.Fatalf("author media import: %+v err=%v", owned, err)
	}
	catalog, err := store.AcademyAuthorCloudMedia(author)
	if err != nil || len(catalog) != 1 || catalog[0].Title != "Private lecture" {
		t.Fatalf("duplicate import changed first title or created duplicate: %+v err=%v", catalog, err)
	}
	// Persistence must never trim a path AFTER the provider verified it.
	// Otherwise a verified file ending in whitespace could silently resolve
	// to a different unverified cloud file when a learner starts playback.
	if _, err := store.AcademyImportCloudMedia(author, []academy.CloudMedia{
		{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/private/author.mp4 ", Title: "Changed source"},
	}); !errors.Is(err, academy.ErrInvalid) {
		t.Fatalf("noncanonical source identity was persisted: %v", err)
	}
	catalog, err = store.AcademyAuthorCloudMedia(author)
	if err != nil || len(catalog) != 1 || catalog[0].ProviderFileRef != "disk:/private/author.mp4" || catalog[0].Title != "Private lecture" {
		t.Fatalf("invalid import modified previously verified media: %+v err=%v", catalog, err)
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

	// Saving the editor's full draft snapshot must remove omitted VIDEO
	// assignments. Previously the database only replaced lessons present
	// in the incoming items, so removed videos remained attached.
	secondVideo, err := store.AcademyAddLesson(author, course.ID, course.VersionID, module.ID,
		academy.Lesson{Title: "Second video", Kind: academy.Video, Ordinal: 3, MediaAssetRef: "private/pending-two"})
	if err != nil {
		t.Fatal(err)
	}
	twoLessons := []academy.MediaAssignment{
		{LessonID: video.ID, MediaID: owned[0].ID, Ordinal: 1},
		{LessonID: secondVideo.ID, MediaID: owned[0].ID, Ordinal: 1},
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, twoLessons); err != nil {
		t.Fatalf("assign two video lessons: %v", err)
	}
	view, err = store.AcademyAuthorLessonMedia(author, course.ID, course.VersionID)
	if err != nil || len(view) != 2 {
		t.Fatalf("two-lesson readback: %+v err=%v", view, err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, assignment); err != nil {
		t.Fatalf("remove omitted lesson assignment: %v", err)
	}
	view, err = store.AcademyAuthorLessonMedia(author, course.ID, course.VersionID)
	if err != nil || len(view) != 1 || view[0].LessonID != video.ID {
		t.Fatalf("omitted lesson retained a ghost video: %+v err=%v", view, err)
	}
	if err := store.AcademyAssignCloudMedia(outsider, course.ID, course.VersionID, nil); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign identity cleared author's media: %v", err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, nil); err != nil {
		t.Fatalf("clear last draft video assignment: %v", err)
	}
	view, err = store.AcademyAuthorLessonMedia(author, course.ID, course.VersionID)
	if err != nil || len(view) != 0 {
		t.Fatalf("cleared draft retained videos: %+v err=%v", view, err)
	}
	var clearEvents int
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1 AND event_type='academy.media.assigned'",
		"academy/version/"+course.VersionID).Scan(&clearEvents); err != nil {
		t.Fatal(err)
	}
	if err := store.AcademyAssignCloudMedia(author, course.ID, course.VersionID, nil); err != nil {
		t.Fatalf("idempotent clear retry: %v", err)
	}
	var clearedRetryEvents int
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1 AND event_type='academy.media.assigned'",
		"academy/version/"+course.VersionID).Scan(&clearedRetryEvents); err != nil {
		t.Fatal(err)
	}
	if clearedRetryEvents != clearEvents {
		t.Fatalf("cleared snapshot retry emitted duplicate event: before=%d after=%d", clearEvents, clearedRetryEvents)
	}
}

// A cloud reconnect must never reinterpret an already verified disk:/ path.
// This uses real migrated PostgreSQL constraints and the canonical import
// transaction, not an in-memory mock of the connection generation.
func TestAcademyCloudMediaGenerationReconnectionPostgres(t *testing.T) {
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
	if _, err := db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", author); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO academy_cloud_connections(author_id,provider,encrypted_access_token)
VALUES($1::uuid,'YANDEX_DISK',$2)`, author, "encrypted-test-only-placeholder-credential-000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRow(`SELECT generation::text FROM academy_cloud_connections WHERE author_id=$1::uuid`, author).Scan(&original); err != nil {
		t.Fatal(err)
	}
	items := []academy.CloudMedia{{Provider: "YANDEX_DISK", ProviderFileRef: "disk:/same-path.mp4", Title: "Bound video",
		ResourceID: "file-resource-A", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", FileSize: 12345}}
	first, err := store.academyImportCloudMediaWithGeneration(author, items, original)
	if err != nil || len(first) != 1 {
		t.Fatalf("first generation import: %+v err=%v", first, err)
	}
	// Replacing a file at the same path without reconnecting must not
	// mutate the originally verified resource ID or content digest.
	replaced := append([]academy.CloudMedia(nil), items...)
	replaced[0].ResourceID = "file-resource-B"
	replaced[0].SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := store.academyImportCloudMediaWithGeneration(author, replaced, original); !errors.Is(err, academy.ErrState) {
		t.Fatalf("overwritten same-path file was silently rebound: %v", err)
	}
	if _, err := db.Exec(`UPDATE academy_cloud_media SET connection_generation=gen_random_uuid() WHERE id=$1::uuid`, first[0].ID); err == nil {
		t.Fatal("provider identity changed outside immutable import")
	}
	if _, err := db.Exec(`UPDATE academy_cloud_connections SET generation=gen_random_uuid() WHERE author_id=$1::uuid`, author); err != nil {
		t.Fatal(err)
	}
	if _, err := store.academyImportCloudMediaWithGeneration(author, items, original); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("stale OAuth generation was accepted: %v", err)
	}
	var current string
	if err := db.QueryRow(`SELECT generation::text FROM academy_cloud_connections WHERE author_id=$1::uuid`, author).Scan(&current); err != nil {
		t.Fatal(err)
	}
	second, err := store.academyImportCloudMediaWithGeneration(author, replaced, current)
	if err != nil || len(second) != 1 || first[0].ID == second[0].ID {
		t.Fatalf("new OAuth account reused old media identity: %+v %+v err=%v", first, second, err)
	}
	catalog, err := store.AcademyAuthorCloudMedia(author)
	if err != nil || len(catalog) != 2 {
		t.Fatalf("author catalog after reconnect: %+v err=%v", catalog, err)
	}
	activeItems := 0
	for _, item := range catalog {
		if item.Active {
			activeItems++
			if item.ID != second[0].ID {
				t.Fatal("old cloud identity advertised as active")
			}
		}
	}
	if activeItems != 1 {
		t.Fatalf("expected exactly one current OAuth video, got %d", activeItems)
	}
	var active int
	if err := db.QueryRow(`SELECT count(*) FROM academy_cloud_media media
JOIN academy_cloud_connections connection ON connection.author_id=media.author_id
 AND connection.provider=media.provider AND connection.generation=media.connection_generation
WHERE media.author_id=$1::uuid AND media.provider_file_ref=$2`, author, items[0].ProviderFileRef).Scan(&active); err != nil || active != 1 {
		t.Fatalf("reconnected path must have exactly one active source: count=%d err=%v", active, err)
	}
}
