package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/searchdistribution"
)
type searchFixture struct {
	offers []searchdistribution.Offer
	err error
}
func (s searchFixture) PublicTopicOffers(_ string)([]searchdistribution.Offer,error){return s.offers,s.err}
func TestSearchDistributionRequiresRealSupplyForIndexing(t *testing.T){
	a:=searchdistribution.Offer{OfferID:"a",SpecialistID:"s1",PriceMinor:50000,HasAvailableSlot:true}
	b:=searchdistribution.Offer{OfferID:"b",SpecialistID:"s2",PriceMinor:70000,HasAvailableSlot:true}
	for _,tc:=range []struct{offers []searchdistribution.Offer;indexed bool}{
		{nil,false},{[]searchdistribution.Offer{a},false},{[]searchdistribution.Offer{a,b},true},
	}{
		h:=New(Options{SearchDistribution:searchFixture{offers:tc.offers}})
		r:=httptest.NewRecorder();h.ServeHTTP(r,httptest.NewRequest("GET","/v1/public/help/son",nil))
		if r.Code!=http.StatusOK||strings.Contains(r.Body.String(),"\"indexable\":true")!=tc.indexed{
			t.Fatalf("offers=%d code=%d body=%s",len(tc.offers),r.Code,r.Body.String())
		}
	}
	for _,tc:=range []struct{slug string;code int}{
		{"/v1/public/help/invalid",404},{"/v1/public/help/son",503},
	}{
		h:=New(Options{})
		r:=httptest.NewRecorder();h.ServeHTTP(r,httptest.NewRequest("GET",tc.slug,nil))
		if r.Code!=tc.code{t.Fatalf("unconfigured search %s=%d",tc.slug,r.Code)}
	}
	h:=New(Options{SearchDistribution:searchFixture{err:errors.New("storage offline")}})
	r:=httptest.NewRecorder();h.ServeHTTP(r,httptest.NewRequest("GET","/v1/public/help/son",nil))
	if r.Code!=http.StatusServiceUnavailable{t.Fatalf("search fail-open=%d",r.Code)}
}
