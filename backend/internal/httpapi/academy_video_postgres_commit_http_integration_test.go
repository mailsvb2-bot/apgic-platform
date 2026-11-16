package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
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
		// A new HTML5 seek after COMMIT must be denied before opening the
		// provider stream. The same canonical PostgreSQL fingerprint check
		// that guards each active write also gates new requests.
		if !authorized() {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
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

	// Reusing the existing HTTP server proves that the committed generation
	// revocation also rejects a subsequent Range request. No cloud URL or
	// credential may appear in a denied learner response.
	seek, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	seek.Header.Set("Range", "bytes=65536-")
	seekResponse, err := client.Do(seek)
	if err != nil {
		t.Fatal(err)
	}
	defer seekResponse.Body.Close()
	if seekResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked PostgreSQL-backed seek returned %d, want 403", seekResponse.StatusCode)
	}
	if location := seekResponse.Header.Get("Location"); location != "" {
		t.Fatalf("revoked seek exposed redirect location %q", location)
	}
	denied, err := io.ReadAll(seekResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(denied), path) || strings.Contains(string(denied), "encrypted-test-only-placeholder") {
		t.Fatal("revoked seek exposed cloud path or credential")
	}
}

// TestAcademyYandexTransportHarnessStopsAfterCommittedEntitlementRevocation
// exercises the test-only Yandex-origin transport seam against the same
// PostgreSQL-backed learner source and per-write authorization check. It uses
// the paid Academy journey created earlier in the isolated CI database, then
// commits the canonical Commerce refund event while a learner response is
// already open. The provider responses and cloud credential are test doubles;
// this is not production CDN delivery or live OAuth acceptance.
type academyVideoEntitlementRoundTripper func(*http.Request) (*http.Response, error)

func (f academyVideoEntitlementRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type academyVideoTrackedBody struct {
	io.ReadCloser
	closed *atomic.Bool
}

type academyVideoCredentialOverride struct {
	*runtimepostgres.Checker
	author string
	token  string
}

func (s academyVideoCredentialOverride) AcademyReadCloudCredential(actor, provider string) (string, error) {
	if actor != s.author || provider != "YANDEX_DISK" {
		return "", academy.ErrDenied
	}
	return s.token, nil
}

func (b academyVideoTrackedBody) Close() error {
	b.closed.Store(true)
	return b.ReadCloser.Close()
}

// TestAcademyYandexTransportHarnessStopsAfterCommittedEntitlementRevocation
// exercises the package-private Yandex transport test seam, session
// authentication, Yandex metadata verification, protected downloader
// transport and PostgreSQL entitlement recheck. The Yandex HTTP responses and
// cloud credential are deterministic test doubles; the entitlement, course,
// enrollment and refund projection are real PostgreSQL state. Production
// learner delivery remains closed until a certified CDN adapter is wired.
func TestAcademyYandexTransportHarnessStopsAfterCommittedEntitlementRevocation(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL Academy journey required")
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

	const learner = "00000000-0000-0000-0000-00000000b003"
	const product = "00000000-0000-0000-0000-00000000af03"
	var enrollmentID, lessonID, mediaID string
	err = db.QueryRowContext(ctx, `SELECT e.id::text,l.id::text,lm.media_id::text
FROM academy_enrollments e
JOIN store_entitlements t ON t.id=e.entitlement_id
JOIN academy_course_versions v ON v.id=e.version_id AND v.state='PUBLISHED'
JOIN academy_courses c ON c.id=v.course_id AND c.state='PUBLISHED'
JOIN academy_modules m ON m.version_id=v.id
JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
JOIN academy_lesson_media lm ON lm.lesson_id=l.id
WHERE e.identity_id=$1::uuid AND c.product_id=$2::uuid
  AND e.state IN ('ACTIVE','COMPLETED') AND academy_entitlement_is_active(t.id)
ORDER BY l.ordinal,lm.ordinal LIMIT 1`, learner, product).Scan(&enrollmentID, &lessonID, &mediaID)
	if err != nil {
		t.Fatalf("active paid VIDEO enrollment fixture missing; run the verified Academy journey first: %v", err)
	}
	author, provider, path, err := store.AcademyLearnerVideoSource(learner, enrollmentID, lessonID, mediaID)
	if err != nil {
		t.Fatalf("canonical learner source denied before refund: %v", err)
	}
	resourceID, digest, size, err := store.AcademyLearnerVideoFingerprint(author, provider, path, mediaID)
	if err != nil {
		t.Fatalf("verified media fingerprint unavailable before refund: %v", err)
	}
	// The database fixture intentionally stores a non-decryptable placeholder.
	// Inject a scoped test credential while keeping source, fingerprint,
	// entitlement and enrollment reads backed by the real PostgreSQL store.
	token := "ci-protected-oauth-token"
	streamStore := academyVideoCredentialOverride{Checker: store, author: author, token: token}
	if size < 2 {
		t.Fatalf("test video fixture is too small for a two-part stream: %d", size)
	}
	firstSize := int(size / 2)
	if firstSize > 64<<10 {
		firstSize = 64 << 10
	}
	secondSize := int(size) - firstSize
	firstChunk := bytes.Repeat([]byte("a"), firstSize)
	secondChunk := bytes.Repeat([]byte("b"), secondSize)
	releaseSecond := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSecond) }) }
	defer release()

	var yandexCalls, upstreamCalls atomic.Int32
	var upstreamClosed atomic.Bool
	yandexHTTP := &http.Client{Transport: academyVideoEntitlementRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "cloud-api.yandex.net" ||
			req.Header.Get("Authorization") != "OAuth "+token {
			return nil, errors.New("Yandex metadata request did not use the protected provider transport")
		}
		if req.URL.Query().Get("path") != path {
			return nil, errors.New("Yandex metadata request path did not match the assigned media")
		}
		yandexCalls.Add(1)
		header := http.Header{"Content-Type": []string{"application/json"}}
		switch req.URL.Path {
		case "/v1/disk/resources":
			payload, marshalErr := json.Marshal(academy.YandexDiskFile{
				Name: "ci-protected-video.mp4", Path: path, Type: "file",
				MimeType: "video/mp4", Size: size, ResourceID: resourceID, SHA256: digest,
			})
			if marshalErr != nil {
				return nil, marshalErr
			}
			return &http.Response{StatusCode: http.StatusOK, Header: header,
				Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Request: req}, nil
		case "/v1/disk/resources/download":
			payload := []byte(`{"href":"https://downloader.disk.yandex.net/resource?ticket=opaque","method":"GET","templated":false}`)
			return &http.Response{StatusCode: http.StatusOK, Header: header,
				Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)), Request: req}, nil
		default:
			return nil, errors.New("unexpected Yandex API endpoint")
		}
	})}
	upstreamHTTP := &http.Client{Transport: academyVideoEntitlementRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "downloader.disk.yandex.net" ||
			req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" ||
			req.Header.Get("Proxy-Authorization") != "" || req.Header.Get("Range") != "" {
			return nil, errors.New("media upstream received an untrusted URL or credential header")
		}
		upstreamCalls.Add(1)
		reader, writer := io.Pipe()
		go func() {
			if _, writeErr := writer.Write(firstChunk); writeErr != nil {
				_ = writer.CloseWithError(writeErr)
				return
			}
			select {
			case <-releaseSecond:
				_, _ = writer.Write(secondChunk)
			case <-req.Context().Done():
			}
			_ = writer.Close()
		}()
		header := http.Header{
			"Content-Type":   []string{"video/mp4"},
			"Content-Length": []string{strconv.FormatInt(size, 10)},
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header,
			Body:          academyVideoTrackedBody{ReadCloser: reader, closed: &upstreamClosed},
			ContentLength: size, Request: req}, nil
	})}

	sessions, err := newClientSessionManager([]byte(strings.Repeat("k", 32)), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessionCookie, err := sessions.issue(learner)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAcademyVideoStreamWithClients(mux, streamStore, sessions, nil,
		academy.YandexDiskClient{Client: yandexHTTP}, upstreamHTTP)
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	streamPath := "/v1/academy/enrollments/" + enrollmentID + "/lessons/" + lessonID + "/videos/" + mediaID + "/stream"
	request, err := http.NewRequest(http.MethodGet, server.URL+streamPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(sessionCookie)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Location") != "" {
		t.Fatalf("active learner stream returned HTTP %d or exposed redirect %q",
			response.StatusCode, response.Header.Get("Location"))
	}
	first := make([]byte, firstSize)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatalf("first authorized video chunk unavailable: %v", err)
	}
	if !bytes.Equal(first, firstChunk) {
		t.Fatal("first authorized video chunk was altered")
	}

	// Apply the canonical Commerce REFUND in a separate transaction while
	// the test-only Yandex-origin HTTP handler is waiting on the next upstream bytes.
	refund, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var applied bool
	var reason string
	err = refund.QueryRowContext(ctx, `SELECT applied,reason_code
FROM apgic_apply_store_lifecycle_event(
 '00000000-0000-0000-0000-00000000aa64',
 '00000000-0000-0000-0000-00000000aa74',
 '00000000-0000-0000-0000-00000000a601',
 'store-provider-event-refund-4',
 'store-refund-tx-1',
 'subscription-ci-1',
 '00000000-0000-0000-0000-00000000b003',
 '00000000-0000-0000-0000-00000000a902',
 4,'REFUND',10000,'RUB','store-evidence/refund-ci',
 '00000000-0000-0000-0000-00000000aa52',now()+interval '24 minutes'
)`).Scan(&applied, &reason)
	if err != nil {
		_ = refund.Rollback()
		t.Fatal(err)
	}
	if !applied || reason != "STORE_EVENT_APPLIED" {
		_ = refund.Rollback()
		t.Fatalf("canonical refund was not applied: applied=%v reason=%s", applied, reason)
	}
	if err := refund.Commit(); err != nil {
		t.Fatal(err)
	}
	var subscriptionState, entitlementState string
	var lastSequence, appliedCount int
	if err := db.QueryRowContext(ctx, `SELECT subscription_state,entitlement_state,last_provider_sequence,applied_event_count
FROM store_subscription_projections
WHERE provider_instance_id='00000000-0000-0000-0000-00000000a601'
  AND subscription_ref='subscription-ci-1'`).Scan(&subscriptionState, &entitlementState, &lastSequence, &appliedCount); err != nil {
		t.Fatal(err)
	}
	if subscriptionState != "REVOKED" || entitlementState != "REVOKED" || lastSequence != 4 || appliedCount != 2 {
		t.Fatalf("canonical refund projection did not converge: subscription=%s entitlement=%s sequence=%d applied=%d",
			subscriptionState, entitlementState, lastSequence, appliedCount)
	}
	var entitlementEffect, ledgerEffect, ledgerEntryID string
	if err := db.QueryRowContext(ctx, `SELECT entitlement_effect,ledger_effect,ledger_entry_id::text
FROM store_lifecycle_effects
WHERE lifecycle_event_id='00000000-0000-0000-0000-00000000aa64'`).Scan(&entitlementEffect, &ledgerEffect, &ledgerEntryID); err != nil {
		t.Fatal(err)
	}
	if entitlementEffect != "REVOKE" || ledgerEffect != "REFUND" || ledgerEntryID != "00000000-0000-0000-0000-00000000aa52" {
		t.Fatalf("refund did not persist its canonical Entitlement/Finance effects: entitlement=%s ledger=%s entry=%s",
			entitlementEffect, ledgerEffect, ledgerEntryID)
	}
	if academyVideoSourceStillAllowed(store, learner, enrollmentID, lessonID, mediaID,
		author, provider, path) {
		t.Fatal("committed canonical Entitlement refund still authorizes the learner source")
	}

	release()
	remaining, readErr := io.ReadAll(response.Body)
	if len(remaining) != 0 {
		t.Fatalf("refunded learner received %d additional video bytes after COMMIT", len(remaining))
	}
	if readErr == nil {
		t.Fatal("refunded HTTP response completed successfully instead of being interrupted")
	}
	if !upstreamClosed.Load() {
		t.Fatal("revoked Yandex test stream did not close the provider response body")
	}

	seek, err := http.NewRequest(http.MethodGet, server.URL+streamPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	seek.AddCookie(sessionCookie)
	seek.Header.Set("Range", "bytes=65536-")
	seekResponse, err := client.Do(seek)
	if err != nil {
		t.Fatal(err)
	}
	defer seekResponse.Body.Close()
	if seekResponse.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("new seek after refund returned HTTP %d, want 402 (canonical entitlement-required response)", seekResponse.StatusCode)
	}
	if seekResponse.Header.Get("Location") != "" || yandexCalls.Load() != 2 || upstreamCalls.Load() != 1 {
		t.Fatalf("revoked seek reached provider or exposed redirect: yandex=%d upstream=%d location=%q",
			yandexCalls.Load(), upstreamCalls.Load(), seekResponse.Header.Get("Location"))
	}
	denied, err := io.ReadAll(seekResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{path, resourceID, token, "https://downloader.disk.yandex.net"} {
		if strings.Contains(string(denied), secret) {
			t.Fatalf("revoked seek disclosed private media metadata %q", secret)
		}
	}
}
