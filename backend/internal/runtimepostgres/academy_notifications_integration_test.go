package runtimepostgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// TestAcademyNotificationConsentDurableOptOut proves the signed identity's
// preferences survive a runtime restart and a disabled category cannot be
// re-enabled by bypassing the HTTP handler with a direct SQL write.
// It runs only against the isolated CI migration database.
func TestAcademyNotificationConsentDurableOptOut(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated Academy database not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	learner, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	other, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{learner, other} {
		if _, err := db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
			t.Fatal(err)
		}
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.AcademyNotificationPreferences(learner)
	if err != nil || empty.Enabled || empty.LearningUpdates || empty.AssessmentResults || empty.CertificateUpdates {
		t.Fatalf("unconfigured learner must be opted out: %+v %v", empty, err)
	}
	wanted := academy.NotificationPreferences{
		Enabled: true, LearningUpdates: true, AssessmentResults: true,
	}
	got, err := store.AcademySaveNotificationPreferences(learner, wanted)
	if err != nil || got != wanted {
		t.Fatalf("opt-in not saved: %+v %v", got, err)
	}
	foreign, err := store.AcademyNotificationPreferences(other)
	if err != nil || foreign.Enabled || foreign.LearningUpdates || foreign.AssessmentResults || foreign.CertificateUpdates {
		t.Fatalf("other learner inherited consent: %+v %v", foreign, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recovered, err := store.AcademyNotificationPreferences(learner)
	if err != nil || recovered != wanted {
		t.Fatalf("restart lost learner consent: %+v %v", recovered, err)
	}
	_, err = db.Exec(`UPDATE academy_notification_preferences
 SET enabled=false,learning_updates=true WHERE identity_id=$1::uuid`, learner)
	if err == nil {
		t.Fatal("direct SQL bypassed disabled notification consent")
	}
	disabled, err := store.AcademySaveNotificationPreferences(learner, academy.NotificationPreferences{
		Enabled: false, LearningUpdates: true, AssessmentResults: true, CertificateUpdates: true,
	})
	if err != nil || disabled.Enabled || disabled.LearningUpdates || disabled.AssessmentResults || disabled.CertificateUpdates {
		t.Fatalf("opt-out did not clear all categories: %+v %v", disabled, err)
	}
	persisted, err := store.AcademyNotificationPreferences(learner)
	if err != nil || persisted != disabled {
		t.Fatalf("opt-out was not durably persisted: %+v %v", persisted, err)
	}
}
