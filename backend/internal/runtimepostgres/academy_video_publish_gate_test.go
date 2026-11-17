package runtimepostgres

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAcademyVideoPublishDefaultsFailClosed(t *testing.T) {
	name := "APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED"
	before, set := os.LookupEnv(name)
	defer func() {
		if set {
			_ = os.Setenv(name, before)
		} else {
			_ = os.Unsetenv(name)
		}
	}()
	_ = os.Unsetenv(name)
	if academyVideoPublishEnabled() {
		t.Fatal("video publication must be disabled without explicit release approval")
	}
	_ = os.Setenv(name, "TRUE")
	if academyVideoPublishEnabled() {
		t.Fatal("non-canonical enable value accepted")
	}
	_ = os.Setenv(name, "true")
	if !academyVideoPublishEnabled() {
		t.Fatal("explicit enable value rejected")
	}
}

func TestAcademyVideoPublishFlagCannotBypassDirectYandexReleaseAcceptance(t *testing.T) {
	t.Setenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED", "true")
	if !academyVideoPublishEnabled() {
		t.Fatal("test requires operator flag to be enabled")
	}
	// An operator flag without an exact build identity cannot select a
	// independently audited direct-provider acceptance and cannot open VIDEO publication.
	t.Setenv("APGIC_COMMIT_SHA", "")
	if academyReleaseSourceSHA() != "" {
		t.Fatal("missing release source SHA was accepted")
	}
	t.Setenv("APGIC_COMMIT_SHA", "not-a-commit")
	if academyReleaseSourceSHA() != "" {
		t.Fatal("malformed release source SHA was accepted")
	}
	validSHA := strings.Repeat("a", 40)
	t.Setenv("APGIC_COMMIT_SHA", validSHA)
	if academyReleaseSourceSHA() != validSHA {
		t.Fatal("valid exact release source SHA was rejected")
	}
}

func TestDirectYandexReleaseAcceptanceRequiresDurableApproval(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("migrated Academy PostgreSQL integration database is unavailable")
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Neither the presence of code nor an operator's environment flag can
	// manufacture an independently audited acceptance for a new commit.
	t.Setenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED", "true")
	t.Setenv("APGIC_COMMIT_SHA", strings.Repeat("e", 40))
	var certified bool
	err = store.db.QueryRowContext(context.Background(),
		"SELECT academy_video_direct_yandex_acceptance_available($1)",
		academyReleaseSourceSHA()).Scan(&certified)
	if err != nil {
		t.Fatalf("direct Yandex release acceptance migration missing: %v", err)
	}
	if certified {
		t.Fatal("unapproved commit unexpectedly certified for direct video publication")
	}
}

func TestAcademyVideoDeliveryModeSQLFailsClosedWithoutApproval(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("migrated Academy PostgreSQL integration database is unavailable")
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, mode := range []string{"", "direct", "cdn", "unsupported"} {
		if _, err := tx.ExecContext(ctx,
			"SELECT set_config('apgic.academy_video_delivery_mode',$1,true)",
			mode); err != nil {
			t.Fatal(err)
		}
		var accepted bool
		if err := tx.QueryRowContext(ctx,
			"SELECT academy_video_delivery_release_available($1)",
			strings.Repeat("e", 40)).Scan(&accepted); err != nil {
			t.Fatalf("mode %q: missing unified release predicate: %v", mode, err)
		}
		if accepted {
			t.Fatalf("unapproved release accepted with delivery mode %q", mode)
		}
	}
}
