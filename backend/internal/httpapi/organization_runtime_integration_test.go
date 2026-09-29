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
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/organization"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func TestOrganizationDirectionHTTPPersistsOwnershipAndArchivesInsteadOfDelete(t *testing.T) {
	databaseURL := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("organization integration database not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	key := []byte("organization-runtime-test-key-000000000000")
	handler := New(Options{
		ClientSessionKey: key,
		Organizations:    store,
		Now:              func() time.Time { return now },
	})

	createOrg := httptest.NewRequest(http.MethodPost, "/v1/organizations",
		strings.NewReader(`{"name":"Universal Practice"}`))
	createOrg.Header.Set("content-type", "application/json")
	createOrgRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createOrgRecorder, createOrg)
	if createOrgRecorder.Code != http.StatusCreated {
		t.Fatalf("create organization status=%d body=%s", createOrgRecorder.Code, createOrgRecorder.Body.String())
	}
	var created organization.Snapshot
	if err := json.Unmarshal(createOrgRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Universal Practice" || created.Status != "ACTIVE" {
		t.Fatalf("created organization=%#v", created)
	}
	cookies := createOrgRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != clientSessionCookieName {
		t.Fatalf("organization bootstrap did not issue signed session: %#v", cookies)
	}

	var membershipStatus, ownershipStatus string
	if err := db.QueryRow(`
		SELECT m.status, o.status
		  FROM organization_memberships m
		  JOIN organization_ownerships o
		    ON o.organization_id = m.organization_id
		   AND o.identity_id = m.identity_id
		 WHERE m.organization_id = $1::uuid
	`, created.ID).Scan(&membershipStatus, &ownershipStatus); err != nil {
		t.Fatal(err)
	}
	if membershipStatus != "ACTIVE" || ownershipStatus != "ACTIVE" {
		t.Fatalf("membership=%q ownership=%q", membershipStatus, ownershipStatus)
	}

	createDirection := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+created.ID+"/directions",
		strings.NewReader(`{"name":"Rehabilitation","direction_type":"REHABILITATION"}`),
	)
	createDirection.Header.Set("content-type", "application/json")
	createDirection.AddCookie(cookies[0])
	createDirectionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createDirectionRecorder, createDirection)
	if createDirectionRecorder.Code != http.StatusCreated {
		t.Fatalf("create direction status=%d body=%s", createDirectionRecorder.Code, createDirectionRecorder.Body.String())
	}
	var withDirection organization.Snapshot
	if err := json.Unmarshal(createDirectionRecorder.Body.Bytes(), &withDirection); err != nil {
		t.Fatal(err)
	}
	if len(withDirection.Directions) != 1 {
		t.Fatalf("directions=%#v", withDirection.Directions)
	}
	direction := withDirection.Directions[0]
	if direction.Type != "REHABILITATION" || direction.Status != "ACTIVE" || direction.OrganizationID != created.ID {
		t.Fatalf("direction=%#v", direction)
	}

	archive := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+created.ID+"/directions/"+direction.ID+"/archive",
		nil,
	)
	archive.AddCookie(cookies[0])
	archiveRecorder := httptest.NewRecorder()
	handler.ServeHTTP(archiveRecorder, archive)
	if archiveRecorder.Code != http.StatusOK {
		t.Fatalf("archive direction status=%d body=%s", archiveRecorder.Code, archiveRecorder.Body.String())
	}
	var archived organization.Snapshot
	if err := json.Unmarshal(archiveRecorder.Body.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if len(archived.Directions) != 1 || archived.Directions[0].Status != "ARCHIVED" {
		t.Fatalf("archived organization=%#v", archived)
	}

	if _, err := db.Exec(`DELETE FROM organization_directions WHERE id = $1::uuid`, direction.ID); err == nil {
		t.Fatal("organization direction hard delete unexpectedly succeeded")
	}
	var persistedStatus string
	var archivedAt sql.NullTime
	if err := db.QueryRow(`
		SELECT status, archived_at
		  FROM organization_directions
		 WHERE id = $1::uuid
	`, direction.ID).Scan(&persistedStatus, &archivedAt); err != nil {
		t.Fatal(err)
	}
	if persistedStatus != "ARCHIVED" || !archivedAt.Valid {
		t.Fatalf("direction truth not preserved: status=%q archived_at=%v", persistedStatus, archivedAt)
	}
}
