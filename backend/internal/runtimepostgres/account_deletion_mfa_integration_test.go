package runtimepostgres

import (
	"context"
	"encoding/base32"
	"os"
	"strings"
	"testing"
	"time"
)

func TestR5ClientDeletionTOTPIsEncryptedDelayedBoundedAndNonReplayable(t *testing.T) {
	url := os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("deletion factor PostgreSQL proof URL not configured")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner := createDeletionOwner(t, store)
	if _, err = store.SubmitAccountDeletionRequest(ctx, owner, "WEB"); err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	copy(key[:], strings.Repeat("k", 32))
	now := time.Now().UTC().Truncate(time.Second)
	secret, status, err := store.EnrollClientDeletionFactor(ctx, owner, key, now.Add(-23*time.Hour))
	if err != nil || !status.Enrolled || len(secret) < 20 {
		t.Fatalf("enrollment failed: %+v %v", status, err)
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	otp := financeMFACode(decoded, now.Unix()/30)
	if _, err = store.VerifyClientDeletionFactor(ctx, owner, "session:"+strings.Repeat("a", 48), otp, key, now); err != ErrDeletionFactorCoolingDown {
		t.Fatalf("unexpired enrollment cooldown bypassed: %v", err)
	}
	// The real API supplies its own clock. An isolated test can move forward
	// without reducing production's mandatory 24-hour wait.
	future := now.Add(2 * time.Hour)
	otp = financeMFACode(decoded, future.Unix()/30)
	verified, err := store.VerifyClientDeletionFactor(ctx, owner, "session:"+strings.Repeat("a", 48), otp, key, future)
	if err != nil || !verified {
		t.Fatalf("eligible TOTP rejected: %v", err)
	}
	if verified, err = store.VerifyClientDeletionFactor(ctx, owner, "session:"+strings.Repeat("a", 48), otp, key, future); err == nil || verified {
		t.Fatal("single-use TOTP code replayed")
	}
	if active, err := store.DeletionFactorActiveAt(ctx, owner, future); err != nil || !active {
		t.Fatalf("verified owner factor unexpectedly revoked: %v %v", active, err)
	}
	var encrypted []byte
	if err = store.db.QueryRowContext(ctx, "SELECT encrypted_secret FROM client_deletion_mfa_factors WHERE identity_id=$1::uuid", owner).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if len(encrypted) <= len(decoded) || strings.Contains(string(encrypted), secret) {
		t.Fatal("deletion possession secret stored in plaintext")
	}
	if _, _, err = store.EnrollClientDeletionFactor(ctx, owner, key, now); err != ErrDeletionFactorExists {
		t.Fatalf("existing factor was silently replaced: %v", err)
	}
}
