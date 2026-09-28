package runtimepostgres

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
)

func TestLegalAcceptanceHTTPPreservesVersionHistory(t *testing.T) {
	databaseURL := os.Getenv("APGIC_LEGAL_ACCEPTANCE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("legal acceptance integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := []byte(strings.Repeat("l", 32))
	handler := httpapi.New(httpapi.Options{
		Demand:           demand.NewConformanceService(nil),
		LegalAcceptances: store,
		ClientSessionKey: key,
	})

	createSession := httptest.NewRequest(
		http.MethodPost,
		"/v1/help-intents",
		strings.NewReader(`{"free_text":"нужна консультация"}`),
	)
	createSession.Header.Set("content-type", "application/json")
	sessionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(sessionRecorder, createSession)
	if sessionRecorder.Code != http.StatusCreated {
		t.Fatalf("session bootstrap status=%d body=%s", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, candidate := range sessionRecorder.Result().Cookies() {
		if candidate.Name == "__Host-apgic_session" {
			sessionCookie = candidate
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("trusted session cookie missing")
	}

	record := func(version, evidenceHash string) legal.Acceptance {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/legal-acceptances",
			strings.NewReader(`{"document_id":"terms","document_version":"`+version+`","evidence_hash":"`+evidenceHash+`"}`),
		)
		request.Header.Set("content-type", "application/json")
		request.AddCookie(sessionCookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated && recorder.Code != http.StatusOK {
			t.Fatalf("record %s status=%d body=%s", version, recorder.Code, recorder.Body.String())
		}
		var acceptance legal.Acceptance
		if err := json.Unmarshal(recorder.Body.Bytes(), &acceptance); err != nil {
			t.Fatal(err)
		}
		return acceptance
	}

	status := func(version string) bool {
		request := httptest.NewRequest(
			http.MethodGet,
			"/v1/legal-acceptances/terms/"+version,
			nil,
		)
		request.AddCookie(sessionCookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status %s code=%d body=%s", version, recorder.Code, recorder.Body.String())
		}
		var body struct {
			Accepted bool `json:"accepted"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Accepted
	}

	v1 := record("v1", "sha256:terms-v1")
	if !status("v1") {
		t.Fatal("accepted v1 was not readable")
	}
	if status("v2") {
		t.Fatal("v2 became accepted without explicit acceptance")
	}

	replay := record("v1", "sha256:terms-v1")
	if replay.ID != v1.ID {
		t.Fatalf("idempotent v1 replay changed evidence id: first=%s replay=%s", v1.ID, replay.ID)
	}

	v2 := record("v2", "sha256:terms-v2")
	if v2.ID == v1.ID {
		t.Fatal("v2 acceptance reused v1 evidence id")
	}
	if !status("v1") || !status("v2") {
		t.Fatal("historical v1 or explicit v2 acceptance missing")
	}

	var versionCount int
	if err := store.db.QueryRow(
		`SELECT count(*)
		   FROM legal_acceptances
		  WHERE identity_id = $1::uuid
		    AND document_id = 'terms'`,
		v1.IdentityID,
	).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 {
		t.Fatalf("versioned acceptance count=%d want=2", versionCount)
	}

	if _, err := store.db.Exec(
		`UPDATE legal_acceptances
		    SET document_version = 'v3'
		  WHERE id = $1::uuid`,
		v1.ID,
	); err == nil {
		t.Fatal("historical legal acceptance mutation was not blocked")
	}
}
