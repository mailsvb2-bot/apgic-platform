package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type signedFact struct {
	ConnectorInstanceID string `json:"connector_instance_id"`
	ExternalEventID string `json:"external_event_id"`
	StreamID string `json:"stream_id"`
	Sequence uint64 `json:"sequence"`
	KeyID string `json:"key_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Payload json.RawMessage `json:"payload"`
	Signature string `json:"signature"`
}
type factForSigning struct {
	ConnectorInstanceID string `json:"connector_instance_id"`
	ExternalEventID string `json:"external_event_id"`
	StreamID string `json:"stream_id"`
	Sequence uint64 `json:"sequence"`
	KeyID string `json:"key_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Payload json.RawMessage `json:"payload"`
}
type roomState struct {
	BookingID string `json:"booking_id"`
	Grants map[string]grantRecord `json:"grants"`
	Seen map[string]bool `json:"seen"`
	Joined map[string]bool `json:"joined"`
	Audio map[string]bool `json:"audio"`
	Finish map[string]bool `json:"finish"`
	Started bool `json:"started"`
	Recovering bool `json:"recovering"`
	Terminal bool `json:"terminal"`
	StartedAt time.Time `json:"started_at"`
	FirstBreakAt time.Time `json:"first_break_at"`
	Sequence uint64 `json:"sequence"`
	Outbox []signedFact `json:"outbox"`
	Delivered int `json:"delivered"`
}
type livekitEvent struct {
	ID string `json:"id"`
	Event string `json:"event"`
	CreatedAt int64 `json:"createdAt"`
	Room struct {Name string `json:"name"`} `json:"room"`
	Participant struct {Identity string `json:"identity"`} `json:"participant"`
	Track struct {Type string `json:"type"`} `json:"track"`
}
type livekitProof struct {
	Issuer string `json:"iss"`
	ExpiresAt int64 `json:"exp"`
	NotBefore int64 `json:"nbf"`
	Hash string `json:"sha256"`
}
func (s *referenceService) readRoom(bookingID string) (*roomState,error) {
	if !validUUID(bookingID) {return nil,errors.New("invalid room identity")}
	file:=filepath.Join(s.config.StateDir,strings.ToLower(bookingID)+".json")
	raw,err:=os.ReadFile(file)
	if os.IsNotExist(err) {
		return &roomState{BookingID:strings.ToLower(bookingID),Grants:map[string]grantRecord{},
			Seen:map[string]bool{},Joined:map[string]bool{},Audio:map[string]bool{},
			Finish:map[string]bool{}},nil
	}
	if err!=nil {return nil,err}
	var room roomState
	if err:=json.Unmarshal(raw,&room);err!=nil {return nil,err}
	if room.BookingID!=strings.ToLower(bookingID) {return nil,errors.New("room identity mismatch")}
	if room.Grants==nil {room.Grants=map[string]grantRecord{}}
	if room.Seen==nil {room.Seen=map[string]bool{}}
	if room.Joined==nil {room.Joined=map[string]bool{}}
	if room.Audio==nil {room.Audio=map[string]bool{}}
	if room.Finish==nil {room.Finish=map[string]bool{}}
	return &room,nil
}
func (s *referenceService) writeRoom(room *roomState) error {
	if err:=os.MkdirAll(s.config.StateDir,0700);err!=nil{return err}
	raw,err:=json.Marshal(room)
	if err!=nil{return err}
	temp,err:=os.CreateTemp(s.config.StateDir,".room-")
	if err!=nil{return err}
	defer os.Remove(temp.Name())
	if err:=temp.Chmod(0600);err!=nil {temp.Close();return err}
	if _,err=temp.Write(raw);err!=nil {temp.Close();return err}
	if err=temp.Sync();err!=nil {temp.Close();return err}
	if err=temp.Close();err!=nil{return err}
	target:=filepath.Join(s.config.StateDir,room.BookingID+".json")
	if err=os.Rename(temp.Name(),target);err!=nil{return err}
	directory,err:=os.Open(s.config.StateDir)
	if err!=nil{return err}
	defer directory.Close()
	return directory.Sync()
}
func (s *referenceService) verifyLiveKitWebhook(body []byte,authorization string) error {
	parts:=strings.Split(strings.TrimSpace(strings.TrimPrefix(authorization,"Bearer ")),".")
	if len(parts)!=3 || len(authorization)>8192{return errors.New("webhook token malformed")}
	header,e:=base64.RawURLEncoding.DecodeString(parts[0])
	if e!=nil{return e}
	var algorithm struct{Alg string `json:"alg"`}
	if json.Unmarshal(header,&algorithm)!=nil || algorithm.Alg!="HS256" {return errors.New("webhook algorithm unsupported")}
	mac:=hmac.New(sha256.New,[]byte(s.config.APISecret))
	_,_=mac.Write([]byte(parts[0]+"."+parts[1]))
	signature,e:=base64.RawURLEncoding.DecodeString(parts[2])
	if e!=nil || !hmac.Equal(signature,mac.Sum(nil)) {return errors.New("webhook signature invalid")}
	claimsRaw,e:=base64.RawURLEncoding.DecodeString(parts[1])
	if e!=nil{return e}
	var claims livekitProof
	if json.Unmarshal(claimsRaw,&claims)!=nil || claims.Issuer!=s.config.APIKey ||
		claims.ExpiresAt<=s.clock().Unix()-5 || claims.NotBefore>s.clock().Unix()+5 {
		return errors.New("webhook claims invalid")
	}
	digest:=sha256.Sum256(body)
	expected,e:=base64.StdEncoding.DecodeString(claims.Hash)
	if e!=nil || !hmac.Equal(expected,digest[:]) {return errors.New("webhook payload hash invalid")}
	return nil
}
func authorizedRole(room *roomState,participant string) (string,bool) {
	for _,g:=range room.Grants {if g.Identity==participant {return g.Role,true}}
	return "",false
}
func (s *referenceService) enqueue(room *roomState,sourceID,factType,role string,occurred time.Time) error {
	room.Sequence++
	if len(room.Outbox)>0 {
		last:=room.Outbox[len(room.Outbox)-1].OccurredAt
		if !occurred.After(last) {occurred=last.Add(time.Millisecond)}
	}
	payload,e:=json.Marshal(map[string]any{
		"booking_id":room.BookingID,"fact_type":factType,"role":role,
		"provider_reference":"livekit/"+roomName(room.BookingID),
		"evidence_ref":"livekit-webhook/"+sourceID+"/"+factType+"/"+role,
	})
	if e!=nil{return e}
	msg:=factForSigning{ConnectorInstanceID:s.config.ProviderID,
		ExternalEventID:"livekit/"+sourceID+"/"+factType+"/"+role,
		StreamID:"consultation/"+room.BookingID,Sequence:room.Sequence,
		KeyID:s.config.KeyID,OccurredAt:occurred.UTC(),Payload:payload}
	raw,e:=json.Marshal(msg)
	if e!=nil{return e}
	signature:=base64.StdEncoding.EncodeToString(ed25519.Sign(s.config.SigningKey,raw))
	room.Outbox=append(room.Outbox,signedFact{ConnectorInstanceID:msg.ConnectorInstanceID,
		ExternalEventID:msg.ExternalEventID,StreamID:msg.StreamID,Sequence:msg.Sequence,
		KeyID:msg.KeyID,OccurredAt:msg.OccurredAt,Payload:payload,Signature:signature})
	return nil
}
func (s *referenceService) applyLiveKitEvent(room *roomState,event livekitEvent) error {
	if room.Terminal || room.Seen[event.ID] {return nil}
	if event.ID=="" || event.CreatedAt<=0 {return errors.New("event identity invalid")}
	occurred:=time.Unix(event.CreatedAt,0).UTC()
	if occurred.After(s.clock().Add(2*time.Minute)) || occurred.Before(s.clock().Add(-7*24*time.Hour)) {
		return errors.New("event time outside review window")
	}
	role,known:=authorizedRole(room,event.Participant.Identity)
	members:=func() (string,string) {
		var client,specialist string
		for _,grant:=range room.Grants {
			if grant.Role=="CLIENT" {client=grant.Identity}
			if grant.Role=="SPECIALIST" {specialist=grant.Identity}
		}
		return client,specialist
	}
	client,specialist:=members()
	switch event.Event {
	case "participant_joined":
		if !known {return errors.New("unknown provider participant")}
		if !room.Joined[event.Participant.Identity] {
			room.Joined[event.Participant.Identity]=true
			if err:=s.enqueue(room,event.ID,"JOINED",role,occurred);err!=nil{return err}
		}
	case "track_published":
		if !known || !room.Joined[event.Participant.Identity] {return errors.New("unauthorized media track")}
		if strings.ToUpper(event.Track.Type)!="AUDIO" {break}
		room.Audio[event.Participant.Identity]=true
		if client!="" && specialist!="" && room.Joined[client] && room.Joined[specialist] &&
			room.Audio[client] && room.Audio[specialist] {
			if !room.Started {
				room.Started=true;room.StartedAt=occurred
				if err:=s.enqueue(room,event.ID,"STARTED","SYSTEM",occurred);err!=nil{return err}
			}else if room.Recovering {
				room.Recovering=false
				if err:=s.enqueue(room,event.ID,"RECOVERY_SUCCEEDED","SYSTEM",occurred);err!=nil{return err}
			}
		}
	case "participant_left":
		if !known {return errors.New("unknown provider participant")}
		if room.Joined[event.Participant.Identity] {
			room.Joined[event.Participant.Identity]=false
			room.Audio[event.Participant.Identity]=false
			if room.Started {
				if room.FirstBreakAt.IsZero(){room.FirstBreakAt=occurred}
				if err:=s.enqueue(room,event.ID,"LEFT",role,occurred);err!=nil{return err}
				if !room.Recovering {
					room.Recovering=true
					if err:=s.enqueue(room,event.ID,"RECOVERY_STARTED","SYSTEM",occurred);err!=nil{return err}
				}
			}
		}
	case "room_finished":
		if !room.Started {break}
		observed:=occurred.Sub(room.StartedAt)
		if !room.FirstBreakAt.IsZero() {observed=room.FirstBreakAt.Sub(room.StartedAt)}
		if observed>=s.config.MinimumSession && client!="" && specialist!="" &&
			room.Finish[client] && room.Finish[specialist] {
			if err:=s.enqueue(room,event.ID,"ENDED","SYSTEM",occurred);err!=nil{return err}
		}else{
			if err:=s.enqueue(room,event.ID,"TECHNICAL_FAILURE","SYSTEM",occurred);err!=nil{return err}
		}
		room.Terminal=true
	case "participant_connection_aborted":
		// There is no authoritative STARTED/ENDED evidence in an aborted
		// connection alone. APGIC must not infer completion or no-show.
	default:
		// room_started, video tracks, recording and arbitrary provider events
		// carry no authority to mutate consultation business truth.
	}
	room.Seen[event.ID]=true
	return nil
}
func (s *referenceService) handleLiveKitWebhook(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodPost {http.Error(w,"method",405);return}
	r.Body=http.MaxBytesReader(w,r.Body,32*1024)
	body,e:=io.ReadAll(r.Body)
	if e!=nil || s.verifyLiveKitWebhook(body,r.Header.Get("Authorization"))!=nil {
		http.Error(w,"provider webhook authentication failed",401);return
	}
	var event livekitEvent
	if json.Unmarshal(body,&event)!=nil || !strings.HasPrefix(event.Room.Name,"apgic-") {
		http.Error(w,"unsupported webhook",400);return
	}
	id:=strings.TrimPrefix(event.Room.Name,"apgic-")
	if !validUUID(id) {http.Error(w,"unknown room",400);return}
	s.mu.Lock()
	room,err:=s.readRoom(id)
	if err==nil && len(room.Grants)==0 {err=errors.New("unissued room")}
	if err==nil {err=s.applyLiveKitEvent(room,event)}
	if err==nil {err=s.writeRoom(room)}
	s.mu.Unlock()
	if err!=nil {http.Error(w,"provider event not accepted",409);return}
	go s.flushRoom(id)
	w.WriteHeader(http.StatusNoContent)
}
func (s *referenceService) flushRoom(id string) {
	s.deliveryMu().Lock()
	defer s.deliveryMu().Unlock()
	for {
		s.mu.Lock()
		state,err:=s.readRoom(id)
		if err!=nil || state.Delivered>=len(state.Outbox) {s.mu.Unlock();return}
		next:=state.Outbox[state.Delivered]
		s.mu.Unlock()
		data,e:=json.Marshal(next);if e!=nil{return}
		ctx,cancel:=context.WithTimeout(context.Background(),4*time.Second)
		req,e:=http.NewRequestWithContext(ctx,http.MethodPost,s.config.APGICWebhookURL,bytes.NewReader(data))
		if e!=nil {cancel();return}
		req.Header.Set("Content-Type","application/json")
		response,e:=s.httpClient.Do(req)
		if e!=nil {cancel();return}
		body,_:=io.ReadAll(io.LimitReader(response.Body,2048))
		response.Body.Close();cancel()
		var ack struct{DeliveryDecision string `json:"delivery_decision"`}
		if response.StatusCode!=200 || json.Unmarshal(body,&ack)!=nil ||
			(ack.DeliveryDecision!="APPLY" && ack.DeliveryDecision!="DUPLICATE") {return}
		s.mu.Lock()
		latest,e:=s.readRoom(id)
		if e==nil && latest.Delivered<len(latest.Outbox) &&
			latest.Outbox[latest.Delivered].ExternalEventID==next.ExternalEventID {
			latest.Delivered++
			e=s.writeRoom(latest)
		}
		s.mu.Unlock()
		if e!=nil{return}
	}
}

// Exactly one sender advances a provider stream across HTTP retries.
var deliveryLock sync.Mutex
func (s *referenceService) deliveryMu() *sync.Mutex {return &deliveryLock}
func (s *referenceService) flushAll() {
	files,e:=os.ReadDir(s.config.StateDir)
	if e!=nil{return}
	for _,file:=range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(),".json"){continue}
		id:=strings.TrimSuffix(file.Name(),".json")
		if validUUID(id) {s.flushRoom(id)}
	}
}
func (s *referenceService) ready() error {
	if _,e:=os.Stat(s.config.ClientBundle);e!=nil{return fmt.Errorf("media client bundle not installed: %w",e)}
	if e:=os.MkdirAll(s.config.StateDir,0700);e!=nil{return e}
	return nil
}
