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

// Proves the canonical media fingerprint resolver observes a committed OAuth
// generation revocation through a separate PostgreSQL connection. This is a
// database boundary test, NOT HTTP playback or Entitlement revocation proof.
func TestAcademyVideoFingerprintRevocationCommitBoundaryPostgres(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	author, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO identities(id) VALUES($1::uuid)", author); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO academy_cloud_connections(author_id,provider,encrypted_access_token)
VALUES($1::uuid,'YANDEX_DISK',$2)`, author, "encrypted-test-only-placeholder-credential-000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	var generation string
	if err := db.QueryRowContext(ctx, `SELECT generation::text FROM academy_cloud_connections
WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	media := academy.CloudMedia{
		Provider: "YANDEX_DISK", ProviderFileRef: "disk:/revocation-commit-boundary.mp4",
		Title: "Commit boundary", ResourceID: "commit-boundary-resource", SHA256: digest, FileSize: 4096,
	}
	imported, err := store.academyImportCloudMediaWithGeneration(author, []academy.CloudMedia{media}, generation)
	if err != nil || len(imported) != 1 {
		t.Fatalf("verified import: %+v err=%v", imported, err)
	}
	check := func() error {
		resource, hash, size, err := store.AcademyLearnerVideoFingerprint(author, media.Provider, media.ProviderFileRef, imported[0].ID)
		if err != nil {
			return err
		}
		if resource != media.ResourceID || hash != digest || size != media.FileSize {
			t.Fatalf("fingerprint changed unexpectedly: resource=%q hash=%q size=%d", resource, hash, size)
		}
		return nil
	}
	if err := check(); err != nil {
		t.Fatalf("pre-revocation source: %v", err)
	}
	// An aborted generation rotation must not revoke media. Exercise a real
	// PostgreSQL rollback before the committed revocation below.
	aborted, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aborted.ExecContext(ctx, `UPDATE academy_cloud_connections SET generation=gen_random_uuid()
WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		_ = aborted.Rollback()
		t.Fatal(err)
	}
	if err := aborted.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := check(); err != nil {
		t.Fatalf("rolled-back generation rotation revoked active media: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE academy_cloud_connections SET generation=gen_random_uuid()
WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		t.Fatal(err)
	}
	// The mutation is not yet committed. An independent connection must not
	// see a speculative revocation or silently switch media generations.
	if err := check(); err != nil {
		t.Fatalf("uncommitted revocation leaked to independent reader: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	resource, hash, size, err := store.AcademyLearnerVideoFingerprint(author, media.Provider, media.ProviderFileRef, imported[0].ID)
	if !errors.Is(err, academy.ErrEntitlement) || resource != "" || hash != "" || size != 0 {
		t.Fatalf("committed revocation still authorized source: resource=%q hash=%q size=%d err=%v", resource, hash, size, err)
	}
	reopened, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resource, hash, size, err = reopened.AcademyLearnerVideoFingerprint(author, media.Provider, media.ProviderFileRef, imported[0].ID)
	if !errors.Is(err, academy.ErrEntitlement) || resource != "" || hash != "" || size != 0 {
		t.Fatalf("fresh connection resurrected revoked source: resource=%q hash=%q size=%d err=%v", resource, hash, size, err)
	}
	// A subsequent author reconnect may import NEW media under the new
	// generation, but must never resurrect media from the revoked generation.
	var currentGeneration string
	if err := db.QueryRowContext(ctx, `SELECT generation::text FROM academy_cloud_connections
WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author).Scan(&currentGeneration); err != nil {
		t.Fatal(err)
	}
	if currentGeneration == generation {
		t.Fatal("committed reconnect generation did not change")
	}
	newMedia := media
	newMedia.ProviderFileRef = "disk:/after-reconnect-boundary.mp4"
	newMedia.ResourceID = "after-reconnect-resource"
	newMedia.Title = "After reconnect"
	// A stale OAuth callback or delayed import from the previous generation
	// must not be able to attach media after the committed reconnect.
	staleMedia := newMedia
	staleMedia.ProviderFileRef = "disk:/stale-generation-boundary.mp4"
	staleMedia.ResourceID = "stale-generation-resource"
	if staleItems, staleErr := reopened.academyImportCloudMediaWithGeneration(author, []academy.CloudMedia{staleMedia}, generation); staleErr == nil || len(staleItems) != 0 {
		t.Fatalf("stale generation imported media after reconnect: items=%+v err=%v", staleItems, staleErr)
	}
	newItems, err := reopened.academyImportCloudMediaWithGeneration(author, []academy.CloudMedia{newMedia}, currentGeneration)
	if err != nil || len(newItems) != 1 {
		t.Fatalf("new generation import: %+v err=%v", newItems, err)
	}
	resource, hash, size, err = reopened.AcademyLearnerVideoFingerprint(author, newMedia.Provider, newMedia.ProviderFileRef, newItems[0].ID)
	if err != nil || resource != newMedia.ResourceID || hash != digest || size != newMedia.FileSize {
		t.Fatalf("new generation source unavailable: resource=%q hash=%q size=%d err=%v", resource, hash, size, err)
	}
	resource, hash, size, err = reopened.AcademyLearnerVideoFingerprint(author, media.Provider, media.ProviderFileRef, imported[0].ID)
	if !errors.Is(err, academy.ErrEntitlement) || resource != "" || hash != "" || size != 0 {
		t.Fatalf("reconnect resurrected old source: resource=%q hash=%q size=%d err=%v", resource, hash, size, err)
	}
}
