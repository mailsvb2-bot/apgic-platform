package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/searchdistribution"
)

type publicSearchIndex struct {
	Pages                    []searchdistribution.Topic `json:"pages"`
	PublicationPolicyVersion string                     `json:"publication_policy_version"`
}

// The sitemap and the visible topic pages use the same server-approved,
// qualification-backed publication gate; no duplicated TypeScript slug list.
func registerSearchDistribution(mux *http.ServeMux, store searchdistribution.Store) {
	load := func(w http.ResponseWriter, r *http.Request, topic searchdistribution.Topic) (searchdistribution.Page, bool) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "SEARCH_PUBLICATION_UNAVAILABLE", "Публичный каталог временно недоступен.", true, nil)
			return searchdistribution.Page{}, false
		}
		offers, err := store.PublicTopicOffers(topic.Code)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "SEARCH_PUBLICATION_UNAVAILABLE", "Публичный каталог временно недоступен.", true, nil)
			return searchdistribution.Page{}, false
		}
		return searchdistribution.Page{
			Topic:                    topic,
			Offers:                   offers,
			Indexable:                searchdistribution.Publishable(offers),
			PublicationPolicyVersion: searchdistribution.PolicyVersion,
		}, true
	}

	mux.HandleFunc("GET /v1/public/help/{slug}", func(w http.ResponseWriter, r *http.Request) {
		topic, known := searchdistribution.TopicForSlug(r.PathValue("slug"))
		if !known {
			writeDemandError(w, r, http.StatusNotFound, "SEARCH_TOPIC_NOT_FOUND", "Направление не найдено.", false, nil)
			return
		}
		page, ok := load(w, r, topic)
		if !ok {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /v1/public/help", func(w http.ResponseWriter, r *http.Request) {
		pages := make([]searchdistribution.Topic, 0)
		for _, slug := range searchdistribution.Slugs() {
			topic, _ := searchdistribution.TopicForSlug(slug)
			page, ok := load(w, r, topic)
			if !ok {
				return // Any unavailable source fails closed; never publish a partial sitemap.
			}
			if page.Indexable {
				pages = append(pages, topic)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, publicSearchIndex{
			Pages:                    pages,
			PublicationPolicyVersion: searchdistribution.PolicyVersion,
		})
	})
}
