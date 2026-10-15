package searchdistribution

import "errors"

const PolicyVersion = "search-publication-v1-qualified-supply"

var ErrInvalidSlug = errors.New("unknown canonical demand slug")

type Topic struct {
	Slug        string `json:"slug"`
	Code        string `json:"topic_code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Offer struct {
	OfferID          string `json:"offer_id"`
	SpecialistID     string `json:"specialist_id"`
	DisplayName      string `json:"display_name"`
	Profession       string `json:"profession"`
	PriceMinor       int64  `json:"price_minor"`
	Currency         string `json:"currency"`
	HasAvailableSlot bool   `json:"has_available_slot"`
}

type Page struct {
	Topic                    Topic   `json:"topic"`
	Offers                   []Offer `json:"offers"`
	Indexable                bool    `json:"indexable"`
	PublicationPolicyVersion string  `json:"publication_policy_version"`
}

type Store interface {
	PublicTopicOffers(topicCode string) ([]Offer, error)
}

// This allowlist is the reviewed, first-party Russian Demand Ontology.
// Neither SEO traffic nor a partner may manufacture an unreviewed topic.
var canonicalTopics = []Topic{
	{
		Slug:        "trevoga",
		Code:        "anxiety",
		Title:       "Помощь при тревоге и стрессе",
		Description: "Подберите специалиста по подтверждённым компетенциям и действующим предложениям.",
	},
	{
		Slug:        "son",
		Code:        "sleep",
		Title:       "Специалисты по вопросам сна",
		Description: "Найдите специалиста и выберите удобное время консультации.",
	},
	{
		Slug:        "otnosheniya",
		Code:        "relationships",
		Title:       "Отношения и общение",
		Description: "Помощь в поиске специалиста для работы с отношениями и коммуникацией.",
	},
	{
		Slug:        "kariera",
		Code:        "career",
		Title:       "Карьера и профессиональное развитие",
		Description: "Проверенные специалисты по карьерным вопросам и развитию.",
	},
}

func TopicForSlug(slug string) (Topic, bool) {
	for _, topic := range canonicalTopics {
		if topic.Slug == slug {
			return topic, true
		}
	}
	return Topic{}, false
}

func Slugs() []string {
	slugs := make([]string, 0, len(canonicalTopics))
	for _, topic := range canonicalTopics {
		slugs = append(slugs, topic.Slug)
	}
	return slugs
}

// Publication is earned, not implied by the existence of a catalog entry.
// This function receives only currently eligible offers from the canonical
// PostgreSQL qualification+product gate. Require distinct available people.
func Publishable(offers []Offer) bool {
	seen := make(map[string]bool)
	for _, offer := range offers {
		if offer.SpecialistID != "" &&
			offer.OfferID != "" &&
			offer.PriceMinor > 0 &&
			offer.Currency == "RUB" &&
			offer.HasAvailableSlot {
			seen[offer.SpecialistID] = true
		}
	}
	return len(seen) >= 2
}
