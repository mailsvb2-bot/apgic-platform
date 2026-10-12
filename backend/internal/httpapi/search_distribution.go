package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/searchdistribution"
)

func registerSearchDistribution(mux *http.ServeMux, store searchdistribution.Store) {
	mux.HandleFunc("GET /v1/public/help/{slug}",func(w http.ResponseWriter,r *http.Request){
		topic,ok:=searchdistribution.TopicForSlug(r.PathValue("slug"))
		if !ok {
			writeDemandError(w,r,http.StatusNotFound,"SEARCH_TOPIC_NOT_FOUND","Направление не найдено.",false,nil)
			return
		}
		if store==nil {
			writeDemandError(w,r,http.StatusServiceUnavailable,"SEARCH_PUBLICATION_UNAVAILABLE","Публичный каталог временно недоступен.",true,nil)
			return
		}
		offers,err:=store.PublicTopicOffers(topic.Code)
		if err!=nil {
			writeDemandError(w,r,http.StatusServiceUnavailable,"SEARCH_PUBLICATION_UNAVAILABLE","Публичный каталог временно недоступен.",true,nil)
			return
		}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,searchdistribution.Page{
			Topic:topic,Offers:offers,Indexable:searchdistribution.Publishable(offers),
			PublicationPolicyVersion:searchdistribution.PolicyVersion,
		})
	})
}
