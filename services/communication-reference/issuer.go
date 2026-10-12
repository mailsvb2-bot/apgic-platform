package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type referenceService struct {
	config config
	mu sync.Mutex
	httpClient *http.Client
	clock func() time.Time
}

type issueRequest struct {
	ContractVersion string `json:"contract_version"`
	ProviderInstanceID string `json:"provider_instance_id"`
	BookingID string `json:"booking_id"`
	IdentityID string `json:"identity_id"`
	Role string `json:"role"`
	Scope string `json:"scope"`
	IdempotencyKey string `json:"idempotency_key"`
	ExpiresAt time.Time `json:"expires_at"`
}
type issueResponse struct {
	ProviderCredentialRef string `json:"provider_credential_ref"`
	JoinURI string `json:"join_uri"`
	ExpiresAt time.Time `json:"expires_at"`
}
type grantRecord struct {
	Identity string `json:"identity"`
	Role string `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedAt time.Time `json:"issued_at"`
	Key string `json:"key"`
}
type jwtClaims struct {
	Issuer string `json:"iss"`
	Subject string `json:"sub"`
	IssuedAt int64 `json:"iat"`
	NotBefore int64 `json:"nbf"`
	ExpiresAt int64 `json:"exp"`
	Video struct {
		RoomJoin bool `json:"roomJoin"`
		Room string `json:"room"`
		CanPublish bool `json:"canPublish"`
		CanSubscribe bool `json:"canSubscribe"`
	} `json:"video"`
}
func roomName(bookingID string) string { return "apgic-"+strings.ToLower(bookingID) }
func identityFor(role, identityID string) string { return role+":"+strings.ToLower(identityID) }
func requestBearer(r *http.Request, secret string) bool {
	value:=r.Header.Get("Authorization")
	want:="Bearer "+secret
	return len(value)==len(want) && subtle.ConstantTimeCompare([]byte(value),[]byte(want))==1
}
func (s *referenceService) signToken(claims jwtClaims) (string,error) {
	header:=base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsBytes,err:=json.Marshal(claims)
	if err!=nil {return "",err}
	body:=header+"."+base64.RawURLEncoding.EncodeToString(claimsBytes)
	h:=hmac.New(sha256.New,[]byte(s.config.APISecret))
	_,_=h.Write([]byte(body))
	return body+"."+base64.RawURLEncoding.EncodeToString(h.Sum(nil)),nil
}
func (s *referenceService) verifyToken(token string) (jwtClaims,error) {
	parts:=strings.Split(token,".")
	if len(parts)!=3 || len(token)>8192 {return jwtClaims{},errors.New("token malformed")}
	header,e:=base64.RawURLEncoding.DecodeString(parts[0])
	if e!=nil {return jwtClaims{},e}
	var meta map[string]any
	if json.Unmarshal(header,&meta)!=nil || meta["alg"]!="HS256" || meta["typ"]!="JWT" {return jwtClaims{},errors.New("token algorithm denied")}
	h:=hmac.New(sha256.New,[]byte(s.config.APISecret))
	_,_=h.Write([]byte(parts[0]+"."+parts[1]))
	signature,e:=base64.RawURLEncoding.DecodeString(parts[2])
	if e!=nil || !hmac.Equal(signature,h.Sum(nil)) {return jwtClaims{},errors.New("token signature invalid")}
	raw,e:=base64.RawURLEncoding.DecodeString(parts[1])
	if e!=nil {return jwtClaims{},e}
	var claims jwtClaims
	if json.Unmarshal(raw,&claims)!=nil || claims.Issuer!=s.config.APIKey ||
		claims.ExpiresAt<=s.clock().Unix() || claims.NotBefore>s.clock().Unix()+3 ||
		claims.Video.RoomJoin!=true || !strings.HasPrefix(claims.Video.Room,"apgic-") {
		return jwtClaims{},errors.New("token claims invalid")
	}
	return claims,nil
}

func (s *referenceService) handleIssue(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	if !requestBearer(r,s.config.APGICIssuerKey) || r.Header.Get("X-APGIC-Contract")!="room-join-v1" {
		http.Error(w,"unauthorized",http.StatusUnauthorized);return
	}
	r.Body=http.MaxBytesReader(w,r.Body,2048)
	var req issueRequest
	decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
	if decoder.Decode(&req)!=nil || req.ContractVersion!="apgic.room-join.v1" ||
		req.ProviderInstanceID!=s.config.ProviderID || !validUUID(req.BookingID) ||
		!validUUID(req.IdentityID) || (req.Role!="CLIENT"&&req.Role!="SPECIALIST") ||
		req.Scope!="ROOM_JOIN" || len(req.IdempotencyKey)<16 || len(req.IdempotencyKey)>128 ||
		strings.TrimSpace(req.IdempotencyKey)!=req.IdempotencyKey {
		http.Error(w,"invalid room join request",http.StatusBadRequest);return
	}
	now:=s.clock().UTC()
	if !req.ExpiresAt.After(now) || req.ExpiresAt.After(now.Add(15*time.Minute)) {
		http.Error(w,"credential lifetime invalid",http.StatusForbidden);return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room,err:=s.readRoom(req.BookingID)
	if err!=nil {http.Error(w,"room state unavailable",http.StatusServiceUnavailable);return}
	if room.Grants==nil {room.Grants=make(map[string]grantRecord)}
	memberID:=identityFor(req.Role,req.IdentityID)
	for _,existing:=range room.Grants {
		if existing.Role==req.Role && existing.Identity!=memberID {
			http.Error(w,"role participant conflict",http.StatusConflict);return
		}
	}
	attempt,found:=room.Grants[req.IdempotencyKey]
	if found && (attempt.Identity!=memberID || attempt.Role!=req.Role || !attempt.ExpiresAt.Equal(req.ExpiresAt)){
		http.Error(w,"idempotency conflict",http.StatusConflict);return
	}
	if !found {
		attempt=grantRecord{Identity:memberID,Role:req.Role,ExpiresAt:req.ExpiresAt.UTC(),IssuedAt:now.Truncate(time.Second),Key:req.IdempotencyKey}
		room.Grants[req.IdempotencyKey]=attempt
		if err:=s.writeRoom(room);err!=nil {http.Error(w,"room persistence unavailable",http.StatusServiceUnavailable);return}
	}
	var claims jwtClaims
	claims.Issuer=s.config.APIKey
	claims.Subject=memberID
	claims.IssuedAt=attempt.IssuedAt.Unix()
	claims.NotBefore=attempt.IssuedAt.Unix()-5
	claims.ExpiresAt=attempt.ExpiresAt.Unix()
	claims.Video.RoomJoin=true
	claims.Video.Room=roomName(req.BookingID)
	claims.Video.CanPublish=true
	claims.Video.CanSubscribe=true
	token,err:=s.signToken(claims)
	if err!=nil {http.Error(w,"issuer failed",http.StatusServiceUnavailable);return}
	ref:=sha256.Sum256([]byte(token))
	join:=s.config.PublicOrigin+"/room/"+strings.ToLower(req.BookingID)+"#token="+url.QueryEscape(token)
	w.Header().Set("Content-Type","application/json")
	w.Header().Set("Cache-Control","no-store")
	w.Header().Set("Referrer-Policy","no-referrer")
	w.WriteHeader(http.StatusCreated)
	_=json.NewEncoder(w).Encode(issueResponse{ProviderCredentialRef:"livekit-grant/sha256/"+hex.EncodeToString(ref[:]),JoinURI:join,ExpiresAt:attempt.ExpiresAt})
}

func (s *referenceService) handleFinish(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	token:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ")
	if token==r.Header.Get("Authorization") {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
	claims,err:=s.verifyToken(token)
	if err!=nil || !strings.HasPrefix(claims.Video.Room,"apgic-"){http.Error(w,"unauthorized",http.StatusUnauthorized);return}
	id:=strings.TrimPrefix(claims.Video.Room,"apgic-")
	if !validUUID(id) {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
	s.mu.Lock()
	defer s.mu.Unlock()
	room,e:=s.readRoom(id)
	if e!=nil {http.Error(w,"state unavailable",503);return}
	authorized:=false
	for _,grant:=range room.Grants {if grant.Identity==claims.Subject {authorized=true;break}}
	if !authorized {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
	if !room.Started || !room.Joined[claims.Subject] {http.Error(w,"media not started",409);return}
	if room.Finish==nil {room.Finish=make(map[string]bool)}
	room.Finish[claims.Subject]=true
	if s.writeRoom(room)!=nil {http.Error(w,"state unavailable",503);return}
	w.Header().Set("Cache-Control","no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *referenceService) handleMediaConfig(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodGet {http.Error(w,"method",405);return}
	w.Header().Set("Cache-Control","no-store")
	w.Header().Set("Content-Type","application/json")
	_=json.NewEncoder(w).Encode(map[string]string{"server_url":s.config.MediaWebSocketURL})
}

func (s *referenceService) handleRoom(w http.ResponseWriter,r *http.Request) {
	id:=strings.TrimPrefix(r.URL.Path,"/room/")
	if !validUUID(id) || r.Method!=http.MethodGet {http.NotFound(w,r);return}
	raw,err:=os.ReadFile("static/room.html")
	if err!=nil {http.Error(w,"media application unavailable",503);return}
	w.Header().Set("Cache-Control","no-store")
	w.Header().Set("Referrer-Policy","no-referrer")
	w.Header().Set("Content-Security-Policy",fmt.Sprintf("default-src 'none'; script-src 'self'; connect-src 'self' %s; media-src 'self' blob:; img-src 'self' blob: data:; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",s.config.MediaWebSocketURL))
	w.Header().Set("Content-Type","text/html; charset=utf-8")
	_,_=w.Write(raw)
}
