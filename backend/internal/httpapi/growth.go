package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/growth"
)

func registerGrowth(mux *http.ServeMux, store growth.Store, key []byte,
	sessions *clientSessionManager,sessionConfigErr error,now func()time.Time) {
	signer,signerErr:=growth.NewSigner(key)
	ready:=func(w http.ResponseWriter,r *http.Request)bool{
		if store==nil || signerErr!=nil || sessionConfigErr!=nil || sessions==nil {
			writeDemandError(w,r,http.StatusServiceUnavailable,"GROWTH_NOT_CONFIGURED","Привлечение по ссылкам ещё не настроено.",true,nil)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /v1/specialist/growth-links",func(w http.ResponseWriter,r *http.Request){
		if !ready(w,r){return}
		owner,ok:=trustedClientIdentity(w,r,sessions,sessionConfigErr,"");if !ok{return}
		var request struct{ OfferID string `json:"offer_id"` }
		decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));decoder.DisallowUnknownFields()
		if err:=decoder.Decode(&request);err!=nil || strings.TrimSpace(request.OfferID)==""{
			writeGrowthError(w,r,http.StatusBadRequest,"GROWTH_LINK_INVALID");return
		}
		expires:=now().UTC().Add(29*24*time.Hour)
		link,err:=store.IssueSpecialistLink(owner,request.OfferID,expires)
		if err!=nil{writeGrowthStoreFailure(w,r,err);return}
		token,err:=signer.Issue(link.ID,link.ExpiresAt)
		if err!=nil{writeGrowthError(w,r,http.StatusServiceUnavailable,"GROWTH_SIGNING_FAILED");return}
		link.Token=token;link.Path="/g/"+token
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusCreated,link)
	})
	mux.HandleFunc("POST /v1/growth/touches",func(w http.ResponseWriter,r *http.Request){
		if !ready(w,r){return}
		var request struct{Token string `json:"token"`}
		decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));decoder.DisallowUnknownFields()
		if err:=decoder.Decode(&request);err!=nil {writeGrowthError(w,r,http.StatusBadRequest,"GROWTH_TOKEN_INVALID");return}
		ref,err:=signer.Verify(request.Token,now().UTC())
		if err!=nil{writeGrowthError(w,r,http.StatusBadRequest,"GROWTH_TOKEN_INVALID");return}
		identity,cookie,err:=sessions.identityForCreate(r)
		if err!=nil{writeGrowthError(w,r,http.StatusUnauthorized,"CLIENT_SESSION_INVALID");return}
		touch,err:=store.CaptureTouch(identity,ref,now().UTC())
		if err!=nil{writeGrowthStoreFailure(w,r,err);return}
		if cookie!=nil{http.SetCookie(w,cookie)}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,touch)
	})
	mux.HandleFunc("POST /v1/specialist/growth-links/{id}/revoke",func(w http.ResponseWriter,r *http.Request){
		if !ready(w,r){return}
		owner,ok:=trustedClientIdentity(w,r,sessions,sessionConfigErr,"");if !ok{return}
		if err:=store.RevokeSpecialistLink(owner,r.PathValue("id"));err!=nil{writeGrowthStoreFailure(w,r,err);return}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,map[string]string{"state":"REVOKED"})
	})
	mux.HandleFunc("GET /v1/specialist/growth-report",func(w http.ResponseWriter,r *http.Request){
		if !ready(w,r){return}
		owner,ok:=trustedClientIdentity(w,r,sessions,sessionConfigErr,"");if !ok{return}
		report,err:=store.SpecialistGrowthReport(owner)
		if err!=nil{writeGrowthStoreFailure(w,r,err);return}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,map[string]any{"funnel":"ACQUISITION_TO_HELP_INTENT","items":report,"payment_attribution_available":false})
	})
}
func writeGrowthError(w http.ResponseWriter,r *http.Request,status int,code string) {
	writeDemandError(w,r,status,code,"Не удалось обработать ссылку распространения.",status>=500,nil)
}
func writeGrowthStoreFailure(w http.ResponseWriter,r *http.Request,err error) {
	switch{
	case errors.Is(err,growth.ErrNotOwner):writeGrowthError(w,r,http.StatusForbidden,"GROWTH_OFFER_NOT_OWNED_OR_ELIGIBLE")
	case errors.Is(err,growth.ErrSelfReferral):writeGrowthError(w,r,http.StatusForbidden,"GROWTH_SELF_REFERRAL")
	case errors.Is(err,growth.ErrLinkUnavailable):writeGrowthError(w,r,http.StatusGone,"GROWTH_LINK_UNAVAILABLE")
	default:writeGrowthError(w,r,http.StatusServiceUnavailable,"GROWTH_STORAGE_FAILED")
	}
}
