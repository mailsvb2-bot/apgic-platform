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
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

func TestDurableClientBecomesSpecialistOnSameTrustedIdentity(t *testing.T) {
	url := os.Getenv("APGIC_IDENTITY_HTTP_TEST_DATABASE_URL")
	if url == "" { t.Skip("identity HTTP integration database not configured") }

	store, err := runtimepostgres.Open(context.Background(), url)
	if err != nil { t.Fatal(err) }
	defer store.Close()

	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil { t.Fatal(err) }
	handler := New(Options{
		Demand: service,
		Specialists: store,
		ClientSessionKey: []byte("identity-http-ci-session-key-00000000"),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/help-intents", strings.NewReader(`{"free_text":"sleep help"}`))
	req.Header.Set("content-type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated { t.Fatalf("help intent status=%d body=%s", rec.Code, rec.Body.String()) }

	var intent demand.Intent
	if err := json.Unmarshal(rec.Body.Bytes(), &intent); err != nil { t.Fatal(err) }
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == clientSessionCookieName { session = c; break }
	}
	if session == nil || intent.ClientIdentityID == "" { t.Fatal("client identity/session missing") }

	req = httptest.NewRequest(http.MethodPut, "/v1/specialist/profile", strings.NewReader(`{"display_name":"Anna Test","profession_code":"PSYCHOLOGIST"}`))
	req.Header.Set("content-type", "application/json")
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("specialist status=%d body=%s", rec.Code, rec.Body.String()) }

	var profile specialist.Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil { t.Fatal(err) }
	if profile.IdentityID != intent.ClientIdentityID {
		t.Fatalf("identity split client=%s specialist=%s", intent.ClientIdentityID, profile.IdentityID)
	}

	roles, version, err := store.IdentityRoles(intent.ClientIdentityID)
	if err != nil { t.Fatal(err) }
	if len(roles) != 2 || string(roles[0]) != "CLIENT" || string(roles[1]) != "SPECIALIST" {
		t.Fatalf("durable roles=%v", roles)
	}
	if version != 2 { t.Fatalf("identity version=%d want=2", version) }

	db, err := sql.Open("pgx", url)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	var identities, profiles int
	if err := db.QueryRow("SELECT count(*) FROM identities WHERE id=$1::uuid", intent.ClientIdentityID).Scan(&identities); err != nil { t.Fatal(err) }
	if err := db.QueryRow("SELECT count(*) FROM specialist_profiles WHERE identity_id=$1::uuid", intent.ClientIdentityID).Scan(&profiles); err != nil { t.Fatal(err) }
	if identities != 1 || profiles != 1 {
		t.Fatalf("account truth split identities=%d profiles=%d", identities, profiles)
	}
}
