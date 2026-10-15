package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/searchdistribution"
)

type searchFixture struct {
	offers []searchdistribution.Offer
	err    error
}

func (s searchFixture) PublicTopicOffers(_ string) ([]searchdistribution.Offer, error) {
	return s.offers, s.err
}

func TestSearchPublicationMatchesCanonicalSitemapPolicy(t *testing.T) {
	first := searchdistribution.Offer{
		OfferID: "offer-a", SpecialistID: "s1", PriceMinor: 50000,
		Currency: "RUB", HasAvailableSlot: true,
	}
	second := searchdistribution.Offer{
		OfferID: "offer-b", SpecialistID: "s2", PriceMinor: 70000,
		Currency: "RUB", HasAvailableSlot: true,
	}
	cases := []struct {
		name     string
		offers   []searchdistribution.Offer
		expected bool
	}{
		{name: "empty", offers: nil},
		{name: "one expert", offers: []searchdistribution.Offer{first}},
		{name: "duplicates", offers: []searchdistribution.Offer{first, first}},
		{name: "qualified supply", offers: []searchdistribution.Offer{first, second}, expected: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(Options{SearchDistribution: searchFixture{offers: tc.offers}})
			detail := httptest.NewRecorder()
			h.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/v1/public/help/son", nil))
			if detail.Code != http.StatusOK {
				t.Fatalf("detail status=%d, body=%s", detail.Code, detail.Body.String())
			}
			var page searchdistribution.Page
			if err := json.Unmarshal(detail.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Indexable != tc.expected || page.Topic.Code != "sleep" {
				t.Fatalf("unexpected detail publication: %#v", page)
			}
			if detail.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("volatile qualified supply must not be cached")
			}
			list := httptest.NewRecorder()
			h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v1/public/help", nil))
			if list.Code != http.StatusOK {
				t.Fatalf("sitemap endpoint status=%d, body=%s", list.Code, list.Body.String())
			}
			var index publicSearchIndex
			if err := json.Unmarshal(list.Body.Bytes(), &index); err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.expected {
				want = len(searchdistribution.Slugs())
			}
			if len(index.Pages) != want {
				t.Fatalf("SEO index had %d pages, want %d", len(index.Pages), want)
			}
			if list.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("SEO index must use live publication status")
			}
		})
	}
}

func TestSearchDistributionDoesNotIndexUnknownOrUnavailableOffers(t *testing.T) {
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/v1/public/help/invalid", http.StatusNotFound},
		{"/v1/public/help/son", http.StatusServiceUnavailable},
		{"/v1/public/help", http.StatusServiceUnavailable},
	} {
		h := New(Options{})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.want {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
	h := New(Options{SearchDistribution: searchFixture{err: errors.New("offline")}})
	for _, path := range []string{"/v1/public/help/son", "/v1/public/help"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "SEARCH_PUBLICATION_UNAVAILABLE") {
			t.Fatalf("fail-open search path=%s status=%d", path, rec.Code)
		}
	}
}
