package runtimepostgres

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// An isolated migrated PostgreSQL is required. Proves restart-safe,
// atomic single claim of one identity by one newly approved browser.
func TestAccountDevicePairingPostgresOneTimeClaim(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("migrated PostgreSQL required")
	}
	ctx := context.Background()
	a, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	actor, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	pairID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.db.ExecContext(ctx, "INSERT INTO identities(id) VALUES($1::uuid)", actor); err != nil {
		t.Fatal(err)
	}
	codeDigest := strings.Repeat("a", 64)
	proofDigest := strings.Repeat("b", 64)
	if err = a.CreateAccountDevicePair(pairID, codeDigest, proofDigest, time.Now().Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := b.ClaimAccountDevicePair(pairID, proofDigest); err != nil || ok {
		t.Fatalf("unapproved claim ok=%v err=%v", ok, err)
	}
	if ok, err := a.ApproveAccountDevicePair(strings.Repeat("c", 64), actor); err != nil || ok {
		t.Fatalf("wrong code ok=%v err=%v", ok, err)
	}
	if ok, err := a.ApproveAccountDevicePair(codeDigest, actor); err != nil || !ok {
		t.Fatalf("approve ok=%v err=%v", ok, err)
	}
	if ok, err := b.ApproveAccountDevicePair(codeDigest, actor); err != nil || ok {
		t.Fatalf("double approve ok=%v err=%v", ok, err)
	}
	if _, ok, err := a.ClaimAccountDevicePair(pairID, strings.Repeat("c", 64)); err != nil || ok {
		t.Fatalf("wrong proof ok=%v err=%v", ok, err)
	}
	type result struct {
		actor string
		ok    bool
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, store := range []*Checker{a, b} {
		wg.Add(1)
		go func(c *Checker) {
			defer wg.Done()
			identity, ok, err := c.ClaimAccountDevicePair(pairID, proofDigest)
			results <- result{identity, ok, err}
		}(store)
	}
	wg.Wait()
	close(results)
	successes := 0
	for outcome := range results {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.ok {
			successes++
			if outcome.actor != actor {
				t.Fatalf("wrong owner %q", outcome.actor)
			}
		}
	}
	if successes != 1 {
		t.Fatalf("one-time claim count=%d want 1", successes)
	}
	if identity, ok, err := b.ClaimAccountDevicePair(pairID, proofDigest); err != nil || ok || identity != "" {
		t.Fatalf("replay after restart-like second connection ok=%v identity=%q err=%v", ok, identity, err)
	}
	// A paired session must be private to the same identity and revocable
	// across independent PostgreSQL connections, without a process restart.
	deviceID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	deviceDigest := strings.Repeat("f", 64)
	if err = a.RegisterAccountPairedSession(deviceID, actor, deviceDigest, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	active, err := b.AccountPairedSessionActive(actor, deviceDigest)
	if err != nil || !active {
		t.Fatalf("paired session unavailable: %v %v", active, err)
	}
	otherActor, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	active, err = b.AccountPairedSessionActive(otherActor, deviceDigest)
	if err != nil || active {
		t.Fatalf("foreign identity sees session: %v %v", active, err)
	}
	owned, err := b.ListAccountPairedSessions(actor)
	if err != nil || len(owned) != 1 || owned[0].ID != deviceID {
		t.Fatalf("paired list: %+v %v", owned, err)
	}
	foreign, err := b.ListAccountPairedSessions(otherActor)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign list: %+v %v", foreign, err)
	}
	if ok, err := a.RevokeAccountPairedSession(otherActor, deviceID); err != nil || ok {
		t.Fatalf("foreign revoke: %v %v", ok, err)
	}
	if ok, err := a.RevokeAccountPairedSession(actor, deviceID); err != nil || !ok {
		t.Fatalf("owner revoke: %v %v", ok, err)
	}
	active, err = b.AccountPairedSessionActive(actor, deviceDigest)
	if err != nil || active {
		t.Fatalf("revoked cookie still accepted: %v %v", active, err)
	}
	if ok, err := b.RevokeAccountPairedSession(actor, deviceID); err != nil || ok {
		t.Fatalf("double revoke: %v %v", ok, err)
	}
	expiredID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if err = a.CreateAccountDevicePair(expiredID, strings.Repeat("d", 64), strings.Repeat("e", 64), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// Force expiry with a SQL update instead of sleeping.
	if _, err = a.db.ExecContext(ctx, "UPDATE account_device_pairings SET created_at=now()-interval '2 minutes', expires_at=now()-interval '1 minute' WHERE id=$1::uuid", expiredID); err != nil {
		t.Fatal(err)
	}
	if ok, err := b.ApproveAccountDevicePair(strings.Repeat("d", 64), actor); err != nil || ok {
		t.Fatalf("expired request approved ok=%v err=%v", ok, err)
	}
}
