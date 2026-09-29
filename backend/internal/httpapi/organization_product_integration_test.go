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

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"

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

	legacyProductID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO products (
			id, owner_type, owner_id, commercial_owner_ref,
			revenue_beneficiary_ref, author_refs
		) VALUES (
			$1::uuid, 'ORGANIZATION', $2::uuid, $3, $4, ARRAY[$4]::text[]
		)
	`, legacyProductID, org.ID, "organization/"+org.ID, "identity/legacy-author"); err != nil {
		t.Fatalf("insert legacy draft product: %v", err)
	}
	var ownerIdentityID string
	if err := db.QueryRow(`
		SELECT identity_id::text
		  FROM organization_ownerships
		 WHERE organization_id = $1::uuid
		   AND status = 'ACTIVE'
		 LIMIT 1
	`, org.ID).Scan(&ownerIdentityID); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.OrganizationProduct(ownerIdentityID, org.ID, legacyProductID)
	if err != nil {
		t.Fatalf("read legacy draft product: %v", err)
	}
	if legacy.Name != "" || legacy.OrganizationDirectionID != "" || legacy.Status != commerce.ProductDraft {
		t.Fatalf("legacy product compatibility=%#v", legacy)
	}

	secondBody := strings.Replace(body, "Initial consultation", "Second consultation", 1)
	createSecond := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+org.ID+"/products",
		strings.NewReader(secondBody),
	)
	createSecond.Header.Set("content-type", "application/json")
	createSecond.AddCookie(cookies[0])
	createSecondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createSecondRecorder, createSecond)
	if createSecondRecorder.Code != http.StatusCreated {
		t.Fatalf("create second product status=%d body=%s", createSecondRecorder.Code, createSecondRecorder.Body.String())
	}
	var secondProduct commerce.ProductSnapshot
	if err := json.Unmarshal(createSecondRecorder.Body.Bytes(), &secondProduct); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE organization_directions
		   SET status = 'ARCHIVED', archived_at = $2
		 WHERE id = $1::uuid
	`, directionID, now); err != nil {
		t.Fatalf("archive direction before publish: %v", err)
	}

	publishArchived := httptest.NewRequest(
		http.MethodPost,
		"/v1/organizations/"+org.ID+"/products/"+secondProduct.ID+"/publish",
		nil,
	)
	publishArchived.AddCookie(cookies[0])
	publishArchivedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publishArchivedRecorder, publishArchived)
	if publishArchivedRecorder.Code != http.StatusConflict ||
		!strings.Contains(publishArchivedRecorder.Body.String(), "PRODUCT_DIRECTION_INVALID") {
		t.Fatalf("publish archived direction status=%d body=%s", publishArchivedRecorder.Code, publishArchivedRecorder.Body.String())
	}
	var archivedAuditCount int
	if err := db.QueryRow(`
		SELECT count(*)
		  FROM audit_records
		 WHERE resource_ref = $1
		   AND action = 'product.published'
	`, secondProduct.ID).Scan(&archivedAuditCount); err != nil {
		t.Fatal(err)
	}
	if archivedAuditCount != 0 {
		t.Fatalf("archived direction publication audit count=%d", archivedAuditCount)
	}
}
