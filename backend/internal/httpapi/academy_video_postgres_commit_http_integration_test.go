package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

// TestAcademyVideoHTTPPerWriteDeniesCommittedPostgresRevocation proves the
// canonical PostgreSQL fingerprint check is used on each video write.
// The first chunk is already delivered over a real HTTP socket. A rolled-back
// cloud generation change must NOT revoke it. Once the change commits, the
// copier must deny the second chunk without relying on a periodic watcher.
// This still does not exercise live Yandex credentials, OAuth or a real PSP.
func TestAcademyVideoHTTPPerWriteDeniesCommittedPostgresRevocation(t *testing.T) {
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
	store, err := runtimepostgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var author, mediaID string
	if err := db.QueryRowContext(ctx, "SELECT gen_random_uuid()::text, gen_random_uuid()::text").Scan(&author, &mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO identities(id) VALUES($1::uuid)", author); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec("DELETE FROM academy_cloud_media WHERE id=$1::uuid", mediaID)
		_, _ = db.Exec("DELETE FROM academy_cloud_connections WHERE author_id=$1::uuid AND provider='YANDEX_DISK'", author)
		_, _ = db.Exec("DELETE FROM identities WHERE id=$1::uuid", author)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO academy_cloud_connections(author_id,provider,encrypted_access_token)
VALUES($1::uuid,'YANDEX_DISK',$2)`, author, "encrypted-test-only-placeholder-credential-000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	var generation string
	if err := db.QueryRowContext(ctx, "SELECT generation::text FROM academy_cloud_connections WHERE author_id=$1::uuid", author).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	const path = "disk:/postgres-http-per-write-commit-proof.mp4"
	const provider = "YANDEX_DISK"
	const resourceID = "http-commit-proof-resource"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const chunkSize = 64 << 10
	if _, err := db.ExecContext(ctx, `INSERT INTO academy_cloud_media
(id,author_id,provider,provider_file_ref,title,connection_generation,resource_id,sha256,file_size)
VALUES($1::uuid,$2::uuid,$3,$4,'HTTP commit boundary',$5::uuid,$6,$7,$8)`,
		mediaID, author, provider, path, generation, resourceID, digest, 2*chunkSize); err != nil {
		t.Fatal(err)
	}
	authorized := func() bool {
		return academyVideoFingerprintStillAllowed(store, author, provider, path, mediaID, resourceID, digest, 2*chunkSize)
	}
	if !authorized() {
		t.Fatal("new verified media does not authorize a first chunk")
	}

	releaseSecond := make(chan struct{})
	var secondReleased bool
	handlerDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		go func() {
			if _, err := writer.Write(bytes.Repeat([]byte("a"), chunkSize)); err != nil {
				return
			}
			select {
			case <-releaseSecond:
				_, _ = writer.Write(bytes.Repeat([]byte("b"), chunkSize))
			case <-r.Context().Done():
			}
			_ = writer.Close()
		}()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "131072")
		w.WriteHeader(http.StatusOK)
		err := academyCopyVideoAuthorized(w, reader, authorized)
		if errors.Is(err, errAcademyVideoRevoked) {
			academyCancelRevokedVideoResponse(w, func() { _ = reader.Close() })
		}
		handlerDone <- err
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initial status=%d", resp.StatusCode)
	}
	first := make([]byte, chunkSize)
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatalf("initial video chunk: %v", err)
	}
	if !bytes.Equal(first, bytes.Repeat([]byte("a"), chunkSize)) {
		t.Fatal("initial video chunk altered")
	}

	rolledBack, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rolledBack.ExecContext(ctx, `UPDATE academy_cloud_connections
SET generation=gen_random_uuid() WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		_ = rolledBack.Rollback()
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !authorized() {
		t.Fatal("rolled-back revocation denied the live HTTP video")
	}

	committed, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committed.ExecContext(ctx, `UPDATE academy_cloud_connections
SET generation=gen_random_uuid() WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		_ = committed.Rollback()
		t.Fatal(err)
	}
	if !authorized() {
		_ = committed.Rollback()
		t.Fatal("uncommitted transaction revoked another connection")
	}
	if err := committed.Commit(); err != nil {
		t.Fatal(err)
	}
	if authorized() {
		t.Fatal("committed PostgreSQL generation change still authorizes the HTTP copier")
	}

	// Exactly one final chunk is released after COMMIT. The per-write check
	// must reject it before net/http can buffer or flush it to the learner.
	if !secondReleased {
		close(releaseSecond)
		secondReleased = true
	}
	rest, readErr := io.ReadAll(resp.Body)
	if len(rest) != 0 {
		t.Fatalf("revoked learner received %d bytes after COMMIT", len(rest))
	}
	if readErr == nil {
		t.Fatal("revoked HTTP response completed successfully instead of being interrupted")
	}
	select {
	case copyErr := <-handlerDone:
		if !errors.Is(copyErr, errAcademyVideoRevoked) {
			t.Fatalf("per-write copier did not deny committed revocation: %v", copyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("per-write copier remained running after committed revocation")
	}
	if !strings.Contains(readErr.Error(), "unexpected EOF") && !strings.Contains(readErr.Error(), "EOF") && !strings.Contains(readErr.Error(), "canceled") {
		t.Logf("client observed transport termination: %v", readErr)
	}
}
