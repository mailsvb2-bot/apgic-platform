package searchdistribution

import "testing"

func TestSlugRegistryIsStrictAndStable(t *testing.T){
	for _,slug:=range Slugs(){
		v,ok:=TopicForSlug(slug)
		if !ok||v.Slug!=slug||v.Code==""||v.Title==""{t.Fatalf("invalid slug %q",slug)}
	}
	if _,ok:=TopicForSlug("../admin");ok{t.Fatal("path traversal accepted")}
	if _,ok:=TopicForSlug("TREVoGA");ok{t.Fatal("unreviewed alias accepted")}
}
func TestPublicationGateCountsUniqueAvailableEligiblePeople(t *testing.T){
	a:=Offer{SpecialistID:"a",OfferID:"one",PriceMinor:400000,HasAvailableSlot:true}
	b:=Offer{SpecialistID:"b",OfferID:"two",PriceMinor:500000,HasAvailableSlot:true}
	if Publishable(nil)||Publishable([]Offer{a})||Publishable([]Offer{a,a}){t.Fatal("thin page was indexed")}
	if !Publishable([]Offer{a,b}){t.Fatal("qualified page not indexed")}
	b.HasAvailableSlot=false
	if Publishable([]Offer{a,b}){t.Fatal("unavailable supply indexed")}
}
