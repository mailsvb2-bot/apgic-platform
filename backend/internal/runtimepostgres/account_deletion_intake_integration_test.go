package runtimepostgres

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// Real PostgreSQL integration: submit, retry, restart readback, parallel submit
// and foreign-owner isolation. No provider erasure is fabricated by this test.
func TestAccountDeletionDurableOwnerIsolationAndConcurrency(t *testing.T) {
	url := os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("APGIC_PERSONAL_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{owner, foreign} {
		if _, err = store.db.ExecContext(ctx, "INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.SubmitAccountDeletionRequest(ctx, owner, "WEB")
	if err != nil {
		t.Fatal(err)
	}
	if first.State != "REQUESTED" || first.Idempotent || first.ID == "" || first.Source != "WEB" {
		t.Fatalf("first request falsely reports deletion or wrong state: %#v", first)
	}
	if _, err = store.SubmitAccountDeletionRequest(ctx, owner, "INVALID"); err == nil {
		t.Fatal("untrusted deletion source accepted")
	}
	second, err := store.SubmitAccountDeletionRequest(ctx, owner, "ANDROID")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || !second.Idempotent || second.Source != "WEB" || second.State != "REQUESTED" {
		t.Fatalf("retry created different deletion or overwrote immutable source: %#v", second)
	}
	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := store.SubmitAccountDeletionRequest(ctx, owner, "IOS")
			results <- outcome{id: r.ID, err: e}
		}()
	}
	wg.Wait()
	close(results)
	for got := range results {
		if got.err != nil || got.id != first.ID {
			t.Fatalf("parallel submission was not idempotent: %#v", got)
		}
	}
	var count int
	if err = store.db.QueryRowContext(ctx, "SELECT count(*) FROM delete_account_requests WHERE identity_id=$1::uuid", owner).Scan(&count); err != nil || count != 1 {
		t.Fatalf("parallel requests created duplicates count=%d err=%v", count, err)
	}
	other, err := store.SubmitAccountDeletionRequest(ctx, foreign, "IOS")
	if err != nil || other.ID == first.ID || other.State != "REQUESTED" {
		t.Fatalf("foreign owner request not isolated: %#v %v", other, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	back, err := reopened.ReadAccountDeletionRequest(ctx, owner)
	if err != nil || back.ID != first.ID || back.State != "REQUESTED" || back.Source != "WEB" {
		t.Fatalf("request lost or crossed identities after reconnect: %#v %v", back, err)
	}
	foreignBack, err := reopened.ReadAccountDeletionRequest(ctx, foreign)
	if err != nil || foreignBack.ID != other.ID || foreignBack.ID == back.ID {
		t.Fatalf("foreign owner data disclosed on readback: %#v %v", foreignBack, err)
	}
}
