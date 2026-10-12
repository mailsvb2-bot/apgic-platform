package searchdistribution

import "errors"

const PolicyVersion = "search-publication-v1-qualified-supply"
var ErrInvalidSlug = errors.New("unknown canonical demand slug")

type Topic struct {
	Slug string `json:"slug"`
	Code string `json:"topic_code"`
	Title string `json:"title"`
	Description string `json:"description"`
}
type Offer struct {
	OfferID string `json:"offer_id"`
	SpecialistID string `json:"specialist_id"`
	DisplayName string `json:"display_name"`
	Profession string `json:"profession"`
	PriceMinor int64 `json:"price_minor"`
	Currency string `json:"currency"`
	HasAvailableSlot bool `json:"has_available_slot"`
}
type Page struct {
	Topic Topic `json:"topic"`
	Offers []Offer `json:"offers"`
	Indexable bool `json:"indexable"`
	PublicationPolicyVersion string `json:"publication_policy_version"`
}
type Store interface {
	PublicTopicOffers(topicCode string) ([]Offer,error)
}

// Canonical slugs are an explicit, versioned part of the Russian
// Demand Ontology. New slugs require a reviewed policy release.
var topics = map[string]Topic {
	"trevoga": {Slug:"trevoga",Code:"anxiety",Title:"Помощь при тревоге и стрессе",Description:"Подберите специалиста по подтверждённым компетенциям и действующим предложениям."},
	"son": {Slug:"son",Code:"sleep",Title:"Специалисты по вопросам сна",Description:"Найдите специалиста и выберите удобное время консультации."},
	"otnosheniya": {Slug:"otnosheniya",Code:"relationships",Title:"Отношения и общение",Description:"Помощь в поиске специалиста для работы с отношениями и коммуникацией."},
	"kariera": {Slug:"kariera",Code:"career",Title:"Карьера и профессиональное развитие",Description:"Проверенные специалисты по карьерным вопросам и развитию."},
}
func TopicForSlug(slug string)(Topic,bool){v,ok:=topics[slug];return v,ok}
func Slugs() []string{return []string{"trevoga","son","otnosheniya","kariera"}}

// Indexing is an earned state, not a fake organic-growth claim. Minimum two
// DIFFERENT qualified specialists with live availability, after provider
// truth has passed the database qualification gate.
func Publishable(offers []Offer) bool {
	seen:=make(map[string]bool)
	for _,o:=range offers {
		if o.SpecialistID!="" && o.OfferID!="" && o.PriceMinor>0 && o.HasAvailableSlot {
			seen[o.SpecialistID]=true
		}
	}
	return len(seen)>=2
}
