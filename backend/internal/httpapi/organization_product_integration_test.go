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
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/organization"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func TestOrganizationProductHTTPPublishesExplicitOwnershipSnapshot(t *testing.T) {
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

	now := time.Date(2026, 9, 29, 18, 0, 0, 0, time.UTC)
	handler := New(Options{
		ClientSessionKey: []byte("organization-product-test-key-0000000000"),
		Organizations:    store,
		Products:         store,
		Now:              func() time.Time { return now },
	})

	createOrg := httptest.NewRequest(http.MethodPost, "/v1/organizations",
		strings.NewReader(`{"name":"Product Practice"}`))
	createOrg.Header.Set("content-type", "application/json")
	createOrgRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createOrgRecorder, createOrg)
	if createOrgRecorder.Code != http.StatusCreated {
		t.Fatalf("create organization status=%d body=%s", createOrgRecorder.Code, createOrgRecorder.Body.String())
	}
	var org organization.Snapshot
	if err := json.Unmarshal(createOrgRecorder.Body.Bytes(), &org); err != nil {
		t.Fatal(err)
	}
	cookies := createOrgRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("missing client session cookie: %#v", cookies)
	}

	createDirection := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+org.ID+"/directions",
		strings.NewReader(`{"name":"Consulting","direction_type":"CONSULTING"}`),
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
	directionID := withDirection.Directions[0].ID

	body := `{
		"name":"Initial consultation",
		"direction_id":"` + directionID + `",
		"commercial_owner_ref":"organization/` + org.ID + `",
		"author_refs":["identity/specialist-1"],
		"revenue_beneficiary_ref":"identity/specialist-1"
	}`
	createProduct := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+org.ID+"/products",
		strings.NewReader(body),
	)
	createProduct.Header.Set("content-type", "application/json")
	createProduct.AddCookie(cookies[0])
	createProductRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createProductRecorder, createProduct)
	if createProductRecorder.Code != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", createProductRecorder.Code, createProductRecorder.Body.String())
	}
	var product commerce.ProductSnapshot
	if err := json.Unmarshal(createProductRecorder.Body.Bytes(), &product); err != nil {
		t.Fatal(err)
	}
	if product.Status != commerce.ProductDraft ||
		product.OwnerType != commerce.OwnerOrganization ||
		product.OwnerID != org.ID ||
		product.OrganizationDirectionID != directionID ||
		product.CommercialOwnerRef != "organization/"+org.ID ||
		len(product.AuthorRefs) != 1 ||
		product.RevenueBeneficiaryRef != "identity/specialist-1" {
		t.Fatalf("draft product=%#v", product)
	}

	publish := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+org.ID+"/products/"+product.ID+"/publish",
		nil,
	)
	publish.AddCookie(cookies[0])
	publishRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publishRecorder, publish)
	if publishRecorder.Code != http.StatusOK {
		t.Fatalf("publish product status=%d body=%s", publishRecorder.Code, publishRecorder.Body.String())
	}
	var published commerce.ProductSnapshot
	if err := json.Unmarshal(publishRecorder.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.Status != commerce.ProductPublished || published.PublishedAt == nil || !published.PublishedAt.Equal(now) {
		t.Fatalf("published product=%#v", published)
	}

	var ownerType, ownerID, commercialOwner, revenueBeneficiary, direction string
	var authorsJSON string
	var authors []string
	var status string
	if err := db.QueryRow(`
		SELECT owner_type, owner_id::text, commercial_owner_ref, to_json(author_refs)::text,
		       revenue_beneficiary_ref, organization_direction_id::text, status
		  FROM products
		 WHERE id = $1::uuid
	`, product.ID).Scan(
		&ownerType,
		&ownerID,
		&commercialOwner,
		&authorsJSON,
		&revenueBeneficiary,
		&direction,
		&status,
	); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(authorsJSON), &authors); err != nil {
		t.Fatal(err)
	}
	if ownerType != "ORGANIZATION" || ownerID != org.ID ||
		commercialOwner != "organization/"+org.ID ||
		len(authors) != 1 || authors[0] != "identity/specialist-1" ||
		revenueBeneficiary != "identity/specialist-1" ||
		direction != directionID || status != "PUBLISHED" {
		t.Fatalf("canonical product mismatch owner=%s/%s commercial=%s authors=%v revenue=%s direction=%s status=%s",
			ownerType, ownerID, commercialOwner, authors, revenueBeneficiary, direction, status)
	}

	var auditCount int
	if err := db.QueryRow(`
		SELECT count(*)
		  FROM audit_records
		 WHERE resource_ref = $1
		   AND action = 'product.published'
		   AND reason = 'APGIC-PROD-001'
	`, product.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("publication audit count=%d", auditCount)
	}

	list := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+org.ID+"/products", nil)
	list.AddCookie(cookies[0])
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list products status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed struct {
		Products []commerce.ProductSnapshot `json:"products"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Products) != 1 || listed.Products[0].ID != product.ID || listed.Products[0].Status != commerce.ProductPublished {
		t.Fatalf("listed products=%#v", listed.Products)
	}

	if _, err := db.Exec(`
		UPDATE products SET status = 'PUBLISHED', published_at = now()
		 WHERE id = $1::uuid
	`, product.ID); err != nil {
		t.Fatalf("idempotent database publication validation failed: %v", err)
	}
}
