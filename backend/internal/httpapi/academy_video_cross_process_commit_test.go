package httpapi

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

// The child is an independent OS process: its SQL pool, entitlement checks,
// HTTP handler and transport cannot share the parent's in-memory state.
// This is a PostgreSQL COMMIT -> separate process -> HTTP transport proof.
// The Yandex origin is deliberately replaced by a paused io.Pipe: genuine
// provider OAuth and the full signed learner journey still require live proof.
func TestAcademyVideoCrossProcessRevocationHelper(t *testing.T) {
	if os.Getenv("APGIC_R8_CROSS_PROCESS_CHILD") != "1" {
		return
	}
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	author, mediaID := os.Getenv("APGIC_R8_TEST_AUTHOR"), os.Getenv("APGIC_R8_TEST_MEDIA")
	store, err := runtimepostgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const provider = "YANDEX_DISK"
	const path = "disk:/r8-cross-process-commit-test.mp4"
	const resource = "r8-cross-process-resource"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	allowed := func() bool {
		r, d, n, err := store.AcademyLearnerVideoFingerprint(author, provider, path, mediaID)
		return err == nil && r == resource && d == digest && n == 10
	}
	if !allowed() {
		t.Fatal("child could not authorize the committed PostgreSQL media fixture")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/video", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if !allowed() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		// The same canonical watcher, cancellation closer, guarded copier,
		// and response deadline types as the production video route.
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		stopCloser := academyCloseVideoBodyOnCancel(ctx, reader)
		defer stopCloser()
		deadline := newAcademyVideoResponseDeadline(w)
		deadline.extend(5 * time.Second)
		done := make(chan struct{})
		go func() {
			defer close(done)
			academyWatchVideoEntitlement(ctx, func() {
				cancel()
				deadline.revoke()
			}, 20*time.Millisecond, allowed)
		}()
		defer func() {
			cancel()
			<-done
		}()
		go func() {
			if _, err := writer.Write([]byte("first")); err != nil {
				return
			}
			// A provider that never delivers a second byte must not keep
			// this HTTP request alive after PostgreSQL revocation.
			<-ctx.Done()
		}()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		copyErr := academyCopyVideoAuthorized(
			&academyVideoLearnerWriter{w: w, deadline: deadline},
			reader, func() bool { return ctx.Err() == nil && allowed() },
		)
		if copyErr == errAcademyVideoRevoked {
			cancel()
			deadline.revoke()
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	// The first line is a machine-readable transport address for the parent.
	fmt.Fprintf(os.Stdout, "APGIC_VIDEO_CHILD_READY %s\n", l.Addr().String())
	if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func TestAcademyVideoCommitRevocationStopsOtherProcessHTTP(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var author, mediaID string
	if err := db.QueryRow("SELECT gen_random_uuid()::text, gen_random_uuid()::text").Scan(&author, &mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", author); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec("DELETE FROM academy_cloud_media WHERE id=$1::uuid", mediaID)
		_, _ = db.Exec("DELETE FROM academy_cloud_connections WHERE author_id=$1::uuid AND provider='YANDEX_DISK'", author)
		_, _ = db.Exec("DELETE FROM identities WHERE id=$1::uuid", author)
	}()
	if _, err := db.Exec(`INSERT INTO academy_cloud_connections(author_id,provider,encrypted_access_token)
VALUES($1::uuid,'YANDEX_DISK','encrypted-test-only-placeholder-credential-000000000000000000000')`, author); err != nil {
		t.Fatal(err)
	}
	const path = "disk:/r8-cross-process-commit-test.mp4"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.Exec(`INSERT INTO academy_cloud_media
(id,author_id,provider,provider_file_ref,title,connection_generation,resource_id,sha256,file_size)
SELECT $1::uuid,$2::uuid,'YANDEX_DISK',$3,'Cross process revocation',generation,$4,$5,10
FROM academy_cloud_connections WHERE author_id=$2::uuid AND provider='YANDEX_DISK'`,
		mediaID, author, path, "r8-cross-process-resource", digest); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcademyVideoCrossProcessRevocationHelper$", "-test.v")
	child.Env = append(os.Environ(),
		"APGIC_R8_CROSS_PROCESS_CHILD=1",
		"APGIC_R8_TEST_AUTHOR="+author,
		"APGIC_R8_TEST_MEDIA="+mediaID)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	lines := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "APGIC_VIDEO_CHILD_READY ") {
				lines <- strings.TrimPrefix(sc.Text(), "APGIC_VIDEO_CHILD_READY ")
				return
			}
		}
		close(lines)
	}()
	var addr string
	select {
	case addr = <-lines:
		if addr == "" {
			t.Fatalf("independent gateway did not start: %s", stderr.String())
		}
	case <-ctx.Done():
		t.Fatalf("independent gateway startup timeout: %v", ctx.Err())
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/video")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength != 10 {
		t.Fatalf("unexpected initial response %d length %d", resp.StatusCode, resp.ContentLength)
	}
	first := make([]byte, 5)
	if _, err := io.ReadFull(resp.Body, first); err != nil || string(first) != "first" {
		t.Fatalf("first provider bytes: %q, %v", first, err)
	}
	// A rolled-back change must not revoke another process' connection.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE academy_cloud_connections
SET generation=gen_random_uuid() WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// Observe the live stream from an independent reader while the commit
	// is still absent. A failed rollback must not masquerade as revocation.
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(resp.Body)
		readDone <- readResult{data, err}
	}()
	select {
	case result := <-readDone:
		t.Fatalf("rolled-back change prematurely closed or leaked stream: %q %v", result.data, result.err)
	case <-time.After(100 * time.Millisecond):
		// The stream is still open; only a COMMIT may change its permission.
	}
	if _, err := db.Exec(`UPDATE academy_cloud_connections
SET generation=gen_random_uuid() WHERE author_id=$1::uuid AND provider='YANDEX_DISK'`, author); err != nil {
		t.Fatal(err)
	}
	// The remote process must observe the committed canonical state.
	// Its watcher closes the stalled reader and the HTTP connection.
	start := time.Now()
	select {
	case result := <-readDone:
		if result.err == nil || len(result.data) != 0 {
			t.Fatalf("cross-process revoked stream completed or leaked bytes: rest=%q err=%v", result.data, result.err)
		}
	case <-ctx.Done():
		t.Fatalf("remote process did not terminate revoked HTTP stream: %v", ctx.Err())
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("revoked stream exceeded bounded cross-process recheck: %s", elapsed)
	}
	seek, err := client.Get("http://" + addr + "/video")
	if err != nil {
		t.Fatal(err)
	}
	defer seek.Body.Close()
	if seek.StatusCode != http.StatusForbidden {
		t.Fatalf("new request after COMMIT was not denied: %d", seek.StatusCode)
	}
}
