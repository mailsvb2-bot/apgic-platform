
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
)

type ConsultationJoinStore interface {
	PrepareCommunicationJoin(context.Context,string,string,string,string,string,time.Time) (*communication.JoinPreparation,error)
	RecordCommunicationJoin(context.Context,string,string,string,communication.JoinPreparation,communication.IssuedRoomGrant,time.Time) error
}
type ConsultationJoinOptions struct {
	Store ConsultationJoinStore
	Issuer communication.RoomIssuer
	ProviderInstanceID string
	PolicyVersion string
}

func registerConsultationJoin(mux *http.ServeMux, config ConsultationJoinOptions, sessions *clientSessionManager, sessionErr error, now func()time.Time) {
	mux.HandleFunc("POST /v1/consultations/{bookingID}/join",func(w http.ResponseWriter,r *http.Request){
		if !consultationUUID.MatchString(r.PathValue("bookingID")) {
			writeDemandError(w,r,http.StatusBadRequest,"CONSULT_BOOKING_ID_INVALID","Некорректная запись.",false,nil)
			return
		}
		identity,ok:=trustedClientIdentity(w,r,sessions,sessionErr,"")
		if !ok{return}
		if r.Header.Get("X-APGIC-Request")!="consultation-v1" {
			writeDemandError(w,r,http.StatusForbidden,"COMM_JOIN_ORIGIN_INVALID","Недоверенный запрос подключения.",false,nil)
			return
		}
		if config.Store==nil || config.Issuer==nil || !consultationUUID.MatchString(config.ProviderInstanceID) ||
			strings.TrimSpace(config.PolicyVersion)=="" {
			writeDemandError(w,r,http.StatusServiceUnavailable,"COMM_PROVIDER_UNAVAILABLE","Провайдер консультации пока не подключён.",false,nil)
			return
		}
		key:=strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(key)<16 || len(key)>128 || strings.ContainsAny(key," \t\r\n") {
			writeDemandError(w,r,http.StatusBadRequest,"COMM_JOIN_KEY_INVALID","Нужен уникальный ключ подключения.",false,nil)
			return
		}
		stamp:=now().UTC()
		decision,err:=config.Store.PrepareCommunicationJoin(r.Context(),r.PathValue("bookingID"),identity,
			config.ProviderInstanceID,config.PolicyVersion,key,stamp)
		switch {
		case errors.Is(err,communication.ErrRoomIssuerUnavailable):
			writeDemandError(w,r,http.StatusServiceUnavailable,"COMM_PROVIDER_UNAVAILABLE","Сервис связи не подготовлен.",true,nil);return
		case err!=nil:
			writeDemandError(w,r,http.StatusServiceUnavailable,"COMM_JOIN_UNAVAILABLE","Проверка входа временно недоступна.",true,nil);return
		case decision==nil:
			consultationHidden(w,r);return
		case decision.Decision.Decision!=communication.Allow || decision.Decision.CredentialRequest==nil:
			writeDemandError(w,r,http.StatusForbidden,decision.Decision.ReasonCode,"Вход на консультацию пока недоступен.",false,nil);return
		}
		grant,err:=config.Issuer.IssueRoomJoin(r.Context(),*decision.Decision.CredentialRequest)
		if err!=nil{
			writeDemandError(w,r,http.StatusServiceUnavailable,"COMM_ISSUER_FAILED","Комната связи временно недоступна. Повторная оплата не нужна.",true,nil);return
		}
		if err:=config.Store.RecordCommunicationJoin(r.Context(),r.PathValue("bookingID"),identity,key,*decision,grant,stamp);err!=nil {
			writeDemandError(w,r,http.StatusConflict,"COMM_JOIN_NOT_COMMITTED","Подключение не подтверждено APGIC. Попробуйте ещё раз.",true,nil);return
		}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,map[string]any{
			"booking_id":r.PathValue("bookingID"),
			"provider_instance_id":config.ProviderInstanceID,
			"join_uri":grant.JoinURI,
			"credential_scope":"ROOM_JOIN",
			"credential_expires_at":grant.ExpiresAt,
			"reason_code":communication.ReasonAllowed,
		})
	})
}
