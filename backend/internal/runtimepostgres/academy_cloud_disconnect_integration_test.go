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

// Confirm the real canonical PostgreSQL revoke makes all prior-generation
// assets fail closed without touching another author's connection or media.
func TestAcademyDisconnectYandexRevokesVerifiedMediaPostgres(t *testing.T) {
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
	other, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{author, other} {
		if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, id := range []string{author, other} {
			_, _ = db.Exec("DELETE FROM academy_cloud_media WHERE author_id=$1::uuid", id)
			_, _ = db.Exec("DELETE FROM academy_cloud_connections WHERE author_id=$1::uuid", id)
			_, _ = db.Exec("DELETE FROM identities WHERE id=$1::uuid", id)
		}
	}()
	const secret = "encrypted-test-only-placeholder-credential-000000000000000000000"
	for _, id := range []string{author, other} {
		if _, err := db.Exec(`INSERT INTO academy_cloud_connections(author_id,provider,encrypted_access_token)
VALUES($1::uuid,'YANDEX_DISK',$2)`, id, secret); err != nil {
			t.Fatal(err)
		}
	}
	type mediaFixture struct {
		id     string
		source string
	}
	create := func(actor, path string) mediaFixture {
		t.Helper()
		var generation string
		if err := db.QueryRow(`SELECT generation::text FROM academy_cloud_connections WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, actor).Scan(&generation); err != nil {
			t.Fatal(err)
		}
		items := []academy.CloudMedia{{Provider: "YANDEX_DISK", ProviderFileRef: path, Title: "Verified",
			ResourceID: "resource-1", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", FileSize: 12345}}
		got, err := store.academyImportCloudMediaWithGeneration(actor, items, generation)
		if err != nil || len(got) != 1 {
			t.Fatalf("verified import: %+v %v", got, err)
		}
		return mediaFixture{id: got[0].ID, source: path}
	}
	oldMedia := create(author, "disk:/disconnect-old.mp4")
	otherMedia := create(other, "disk:/other-author.mp4")
	if _, _, _, err = store.AcademyLearnerVideoFingerprint(author, "YANDEX_DISK", oldMedia.source, oldMedia.id); err != nil {
		t.Fatalf("pre-revocation fingerprint: %v", err)
	}
	if err = store.AcademyDisconnectCloud(other, "INVALID_PROVIDER"); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("unexpected provider deleted: %v", err)
	}
	if err = store.AcademyDisconnectCloud(author, "YANDEX_DISK"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, _, _, err = store.AcademyLearnerVideoFingerprint(author, "YANDEX_DISK", oldMedia.source, oldMedia.id); !errors.Is(err, academy.ErrEntitlement) {
		t.Fatalf("disconnected media still playable: %v", err)
	}
	if _, _, _, err = store.AcademyLearnerVideoFingerprint(other, "YANDEX_DISK", otherMedia.source, otherMedia.id); err != nil {
		t.Fatalf("other author's media revoked: %v", err)
	}
	var remaining int
	if err := db.QueryRow("SELECT count(*) FROM academy_cloud_connections WHERE author_id=$1::uuid", author).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("removed author still has credential row: %d %v", remaining, err)
	}
	if err = store.AcademyDisconnectCloud(author, "YANDEX_DISK"); !errors.Is(err, academy.ErrNotFound) {
		t.Fatalf("second disconnect should report not connected: %v", err)
	}
}
