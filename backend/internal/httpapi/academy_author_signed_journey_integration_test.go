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
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/organization"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

// This is the real signed HTTP author journey through existing Identity,
// Organization, Commerce and Academy owners on a migrated PostgreSQL database.
// It intentionally does not simulate a PSP capture or grant an entitlement.
func TestAcademyAuthorSignedCommerceToPublishedCoursePostgres(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := runtimepostgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := New(Options{
		ClientSessionKey: []byte(strings.Repeat("a", 48)),
		Organizations:    store,
		Products:         store,
		Academy:          store,
		Now:              func() time.Time { return time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC) },
	})
	request := func(method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader *strings.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = strings.NewReader(string(raw))
		} else {
			reader = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	require := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("HTTP status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
	}
	decode := func(w *httptest.ResponseRecorder, dst any) {
		t.Helper()
		if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
			t.Fatalf("decode response %s: %v", w.Body.String(), err)
		}
	}

	// The first organization POST bootstraps a real signed Identity cookie.
	orgResponse := request("POST", "/v1/organizations", nil, map[string]any{"name": "Academy signed author"})
	require(orgResponse, http.StatusCreated)
	var org organization.Snapshot
	decode(orgResponse, &org)
	cookies := orgResponse.Result().Cookies()
	if len(cookies) != 1 || org.ID == "" {
		t.Fatalf("missing organization or signed cookie: org=%+v cookies=%v", org, cookies)
	}
	authorCookie := cookies[0]
	directionResponse := request("POST", "/v1/organizations/"+org.ID+"/directions", authorCookie,
		map[string]any{"name": "Education", "direction_type": "EDUCATION"})
	require(directionResponse, http.StatusCreated)
	var directed organization.Snapshot
	decode(directionResponse, &directed)
	if len(directed.Directions) != 1 || directed.Directions[0].Status != "ACTIVE" {
		t.Fatalf("education direction missing: %+v", directed)
	}
	productResponse := request("POST", "/v1/organizations/"+org.ID+"/products", authorCookie, map[string]any{
		"name": "Academy author product", "direction_id": directed.Directions[0].ID,
		"commercial_owner_ref":    "organization/" + org.ID,
		"author_refs":             []string{"identity/original-author"},
		"revenue_beneficiary_ref": "organization/" + org.ID,
	})
	require(productResponse, http.StatusCreated)
	var product commerce.ProductSnapshot
	decode(productResponse, &product)
	if product.ID == "" || product.Status != commerce.ProductDraft {
		t.Fatalf("product not a durable Commerce draft: %+v", product)
	}
	publishedProduct := request("POST", "/v1/organizations/"+org.ID+"/products/"+product.ID+"/publish", authorCookie, nil)
	require(publishedProduct, http.StatusOK)
	decode(publishedProduct, &product)
	if product.Status != commerce.ProductPublished {
		t.Fatalf("Commerce product was not published: %+v", product)
	}
	eligible := request("GET", "/v1/academy/author/eligible-products", authorCookie, nil)
	require(eligible, http.StatusOK)
	var eligibleProducts struct {
		Products []academy.AuthorProduct `json:"products"`
	}
	decode(eligible, &eligibleProducts)
	found := false
	for _, p := range eligibleProducts.Products {
		if p.ID == product.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("published Commerce product absent from signed Academy author picker")
	}

	courseInput := map[string]any{
		"product_id": product.ID, "title": "Signed Academy course",
		"description": "Verified author path", "author_id": "00000000-0000-0000-0000-00000000b003",
	}
	createdResponse := request("POST", "/v1/academy/courses", authorCookie, courseInput)
	require(createdResponse, http.StatusCreated)
	var created academy.Course
	decode(createdResponse, &created)
	if created.ID == "" || created.VersionID == "" || created.ProductID != product.ID || created.State != "DRAFT" {
		t.Fatalf("course did not bind to canonical Commerce product: %+v", created)
	}
	if created.AuthorID == courseInput["author_id"] {
		t.Fatal("forged author_id was trusted instead of the signed cookie")
	}
	retry := request("POST", "/v1/academy/courses", authorCookie, courseInput)
	require(retry, http.StatusCreated)
	var replayed academy.Course
	decode(retry, &replayed)
	if replayed.ID != created.ID || replayed.VersionID != created.VersionID {
		t.Fatalf("lost-response retry created another course: original=%+v retry=%+v", created, replayed)
	}
	mine := request("GET", "/v1/academy/my-courses", authorCookie, nil)
	require(mine, http.StatusOK)
	if !strings.Contains(mine.Body.String(), created.ID) {
		t.Fatalf("durable course missing from author readback: %s", mine.Body.String())
	}

	// The same signed author can publish a text-only original curriculum.
	base := "/v1/academy/courses/" + created.ID + "/versions/" + created.VersionID
	moduleResponse := request("POST", base+"/modules", authorCookie, map[string]any{"title": "Introduction", "ordinal": 1})
	require(moduleResponse, http.StatusCreated)
	var module academy.Module
	decode(moduleResponse, &module)
	lessonResponse := request("POST", base+"/modules/"+module.ID+"/lessons", authorCookie,
		map[string]any{"title": "Reading", "kind": "READING", "ordinal": 1, "body": "Original author content"})
	require(lessonResponse, http.StatusCreated)
	var lesson academy.Lesson
	decode(lessonResponse, &lesson)
	if lesson.ID == "" {
		t.Fatal("lesson not persisted")
	}
	rights := request("POST", base+"/rights/original", authorCookie,
		map[string]any{"provenance_ref": "original/signed-author-curriculum", "acknowledged": true})
	require(rights, http.StatusCreated)
	publish := request("POST", base+"/publish", authorCookie, nil)
	require(publish, http.StatusOK)
	var publishedCourse academy.Course
	decode(publish, &publishedCourse)
	if publishedCourse.State != "PUBLISHED" || publishedCourse.ID != created.ID {
		t.Fatalf("author publication failed: %+v", publishedCourse)
	}
	catalog := request("GET", "/v1/academy/courses", nil, nil)
	require(catalog, http.StatusOK)
	if !strings.Contains(catalog.Body.String(), created.ID) {
		t.Fatalf("published course missing from catalog: %s", catalog.Body.String())
	}

	// A second signed Identity must neither discover the product as eligible
	// nor take over course creation. A published course is NOT a free grant.
	foreignOrg := request("POST", "/v1/organizations", nil, map[string]any{"name": "Foreign Academy author"})
	require(foreignOrg, http.StatusCreated)
	foreignCookies := foreignOrg.Result().Cookies()
	if len(foreignCookies) != 1 {
		t.Fatal("foreign signed session missing")
	}
	foreignCookie := foreignCookies[0]
	foreignEligible := request("GET", "/v1/academy/author/eligible-products", foreignCookie, nil)
	require(foreignEligible, http.StatusOK)
	var foreignProducts struct {
		Products []academy.AuthorProduct `json:"products"`
	}
	decode(foreignEligible, &foreignProducts)
	for _, p := range foreignProducts.Products {
		if p.ID == product.ID {
			t.Fatal("foreign signed Identity discovered another author's product")
		}
	}
	foreignCreate := request("POST", "/v1/academy/courses", foreignCookie, courseInput)
	require(foreignCreate, http.StatusForbidden)
	access := request("GET", "/v1/academy/me/course-access", authorCookie, nil)
	require(access, http.StatusOK)
	if strings.Contains(access.Body.String(), created.ID) {
		t.Fatalf("unpaid author received course access: %s", access.Body.String())
	}
	enroll := request("POST", "/v1/academy/courses/"+created.ID+"/enroll", authorCookie, map[string]any{})
	if enroll.Code >= 200 && enroll.Code < 300 {
		t.Fatalf("unpaid enrollment was accepted: %s", enroll.Body.String())
	}
	var courses, createdEvents, enrollments, verifications int
	if err := db.QueryRow("SELECT count(*) FROM academy_courses WHERE product_id=$1::uuid", product.ID).Scan(&courses); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1 AND event_type='academy.course.created'",
		"academy/course/"+created.ID).Scan(&createdEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM academy_enrollments WHERE version_id=$1::uuid", created.VersionID).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM store_transaction_verifications WHERE product_ref=$1", product.ID).Scan(&verifications); err != nil {
		t.Fatal(err)
	}
	if courses != 1 || createdEvents != 1 || enrollments != 0 || verifications != 0 {
		t.Fatalf("author journey side effects: courses=%d created_events=%d enrollments=%d verifications=%d",
			courses, createdEvents, enrollments, verifications)
	}

	// The newly merged one-click route must use the same signed Identity,
	// persist a private Commerce draft, and never publish or grant access.
	// This route has a strict input schema: forged author_id must be
	// rejected, not silently accepted or trusted.
	forgedDraft := request("POST", "/v1/academy/drafts", authorCookie, map[string]any{
		"title": "One-click signed course", "description": "Original unsold draft",
		"author_id": "00000000-0000-0000-0000-00000000b003",
	})
	require(forgedDraft, http.StatusBadRequest)
	standalone := request("POST", "/v1/academy/drafts", authorCookie, map[string]any{
		"title": "One-click signed course", "description": "Original unsold draft",
	})
	require(standalone, http.StatusCreated)
	var draft academy.Course
	decode(standalone, &draft)
	if draft.ID == "" || draft.ProductID == "" || draft.State != "DRAFT" ||
		draft.AuthorID != created.AuthorID || draft.ProductID == product.ID {
		t.Fatalf("one-click draft did not preserve signed author and private product: %+v", draft)
	}
	var draftProductStatus string
	if err := db.QueryRow("SELECT status FROM products WHERE id=$1::uuid", draft.ProductID).
		Scan(&draftProductStatus); err != nil {
		t.Fatal(err)
	}
	if draftProductStatus != "DRAFT" {
		t.Fatalf("one-click Commerce product unexpectedly published: %s", draftProductStatus)
	}
	ownedDrafts := request("GET", "/v1/academy/my-courses", authorCookie, nil)
	require(ownedDrafts, http.StatusOK)
	if !strings.Contains(ownedDrafts.Body.String(), draft.ID) {
		t.Fatalf("one-click draft absent from signed author readback: %s", ownedDrafts.Body.String())
	}
	publicCatalog := request("GET", "/v1/academy/courses", nil, nil)
	require(publicCatalog, http.StatusOK)
	if strings.Contains(publicCatalog.Body.String(), draft.ID) {
		t.Fatalf("private one-click draft leaked into public catalog: %s", publicCatalog.Body.String())
	}
	draftAccess := request("GET", "/v1/academy/me/course-access", authorCookie, nil)
	require(draftAccess, http.StatusOK)
	if strings.Contains(draftAccess.Body.String(), draft.ID) {
		t.Fatalf("unpaid one-click draft granted access: %s", draftAccess.Body.String())
	}
	draftEnroll := request("POST", "/v1/academy/courses/"+draft.ID+"/enroll", authorCookie, map[string]any{})
	if draftEnroll.Code >= 200 && draftEnroll.Code < 300 {
		t.Fatalf("unpaid one-click draft enrollment accepted: %s", draftEnroll.Body.String())
	}
}
