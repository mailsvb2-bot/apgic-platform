package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

func TestDurableClientBecomesSpecialistOnSameTrustedIdentity(t *testing.T) {
	databaseURL := os.Getenv("APGIC_IDENTITY_HTTP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("identity HTTP integration database not configured")
	}

	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{
		Demand:           service,
		Specialists:      store,
		ClientSessionKey: []byte(strings.Repeat("s", 32)),
	})

	createIntent := httptest.NewRequest(
		http.MethodPost,
		"/v1/help-intents",
		strings.NewReader(`{"free_text":"sleep help"}`),
	)
	createIntent.Header.Set("content-type", "application/json")
	createIntentRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createIntentRecorder, createIntent)
	if createIntentRecorder.Code != http.StatusCreated {
		t.Fatalf(
			"help intent status=%d body=%s",
			createIntentRecorder.Code,
			createIntentRecorder.Body.String(),
		)
	}

	var intent demand.Intent
	if err := json.Unmarshal(createIntentRecorder.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	if intent.ClientIdentityID == "" {
		t.Fatal("help intent did not bind a durable client identity")
	}

	var session *http.Cookie
	for _, candidate := range createIntentRecorder.Result().Cookies() {
		if candidate.Name == clientSessionCookieName {
			session = candidate
			break
		}
	}
	if session == nil {
		t.Fatal("help intent did not issue a trusted client session")
	}

	rolesBefore, versionBefore, err := store.IdentityRoles(intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolesBefore) != 1 || rolesBefore[0] != identity.RoleClient {
		t.Fatalf("initial durable roles=%v want=[CLIENT]", rolesBefore)
	}
	if versionBefore != 1 {
		t.Fatalf("initial identity version=%d want=1", versionBefore)
	}

	upsertProfile := httptest.NewRequest(
		http.MethodPut,
		"/v1/specialist/profile",
		strings.NewReader(`{"display_name":"Anna Test","profession_code":"PSYCHOLOGIST"}`),
	)
	upsertProfile.Header.Set("content-type", "application/json")
	upsertProfile.AddCookie(session)
	upsertProfileRecorder := httptest.NewRecorder()
	handler.ServeHTTP(upsertProfileRecorder, upsertProfile)
	if upsertProfileRecorder.Code != http.StatusOK {
		t.Fatalf(
			"specialist status=%d body=%s",
			upsertProfileRecorder.Code,
			upsertProfileRecorder.Body.String(),
		)
	}

	var profile specialist.Profile
	if err := json.Unmarshal(upsertProfileRecorder.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.IdentityID != intent.ClientIdentityID {
		t.Fatalf(
			"identity split client=%s specialist=%s",
			intent.ClientIdentityID,
			profile.IdentityID,
		)
	}

	rolesAfter, versionAfter, err := store.IdentityRoles(intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolesAfter) != 2 ||
		rolesAfter[0] != identity.RoleClient ||
		rolesAfter[1] != identity.RoleSpecialist {
		t.Fatalf("durable roles=%v want=[CLIENT SPECIALIST]", rolesAfter)
	}
	if versionAfter != 2 {
		t.Fatalf("identity version=%d want=2", versionAfter)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var identities int
	if err := db.QueryRow(
		"SELECT count(*) FROM identities WHERE id=$1::uuid",
		intent.ClientIdentityID,
	).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	var profiles int
	if err := db.QueryRow(
		"SELECT count(*) FROM specialist_profiles WHERE identity_id=$1::uuid",
		intent.ClientIdentityID,
	).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	var roleRows int
	if err := db.QueryRow(
		"SELECT count(*) FROM identity_roles WHERE identity_id=$1::uuid",
		intent.ClientIdentityID,
	).Scan(&roleRows); err != nil {
		t.Fatal(err)
	}
	if identities != 1 || profiles != 1 || roleRows != 2 {
		t.Fatalf(
			"account truth split identities=%d profiles=%d roles=%d",
			identities,
			profiles,
			roleRows,
		)
	}
}
