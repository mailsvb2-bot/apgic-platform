package runtimepostgres

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "strings"
 "time"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)
const liveEventTimeout = 5*time.Second

// liveEventByID queries public business metadata only. Organizer identity is
// required for draft/private reads, never trusted from caller-supplied fields.
func liveEventByID(ctx context.Context, query interface{QueryRowContext(context.Context,string,...any)*sql.Row}, id string, actor string) (liveevents.Event,error) {
 var event liveevents.Event
 err:=query.QueryRowContext(ctx,`SELECT e.id::text,e.organization_id::text,e.organizer_identity_id::text,
 e.title,e.description,e.format,e.starts_at,e.ends_at,e.capacity,e.state,e.price_minor,e.currency,
 e.recording_available,
 (SELECT count(*) FROM live_event_registrations r WHERE r.event_id=e.id AND r.state='REGISTERED')
 FROM live_events e WHERE e.id=$1::uuid AND
 (e.state IN ('PUBLISHED','LIVE','COMPLETED') OR EXISTS
 (SELECT 1 FROM organization_ownerships o WHERE o.organization_id=e.organization_id
  AND o.identity_id=NULLIF($2,'')::uuid AND o.status='ACTIVE'))`,id,actor).Scan(
 &event.ID,&event.OrganizationID,&event.OrganizerIdentityID,&event.Title,&event.Description,
 &event.Format,&event.StartsAt,&event.EndsAt,&event.Capacity,&event.State,
 &event.PriceMinor,&event.Currency,&event.RecordingAvailable,&event.Registered)
 if errors.Is(err,sql.ErrNoRows){return liveevents.Event{},liveevents.ErrNotFound}
 if err!=nil{return liveevents.Event{},fmt.Errorf("live event read: %w",err)}
 if actor=="" {event.OrganizerIdentityID=""}
 return event,nil
}
func (c *Checker) LiveEvent(ctx context.Context, eventID, actor string) (liveevents.Event,error) {
 if c==nil||c.db==nil{return liveevents.Event{},liveevents.ErrUnavailable}
 return liveEventByID(ctx,c.db,eventID,actor)
}
func (c *Checker) ListLiveEvents(ctx context.Context) ([]liveevents.Event,error) {
 if c==nil||c.db==nil{return nil,liveevents.ErrUnavailable}
 rows,err:=c.db.QueryContext(ctx,`SELECT id::text FROM live_events
 WHERE state IN ('PUBLISHED','LIVE') AND ends_at>now()
 ORDER BY starts_at,id LIMIT 100`)
 if err!=nil{return nil,fmt.Errorf("list events: %w",err)}
 defer rows.Close()
 var ids []string
 for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{return nil,err};ids=append(ids,id)}
 if err=rows.Err();err!=nil{return nil,err}
 events:=make([]liveevents.Event,0,len(ids))
 for _,id:=range ids {
  event,err:=c.LiveEvent(ctx,id,"");if err!=nil{return nil,err}
  events=append(events,event)
 }
 return events,nil
}
func (c *Checker) CreateLiveEvent(ctx context.Context,actor,org,idem string,in liveevents.DraftInput,now time.Time)(liveevents.Event,error){
 if c==nil||c.db==nil{return liveevents.Event{},liveevents.ErrUnavailable}
 in,err:=liveevents.NormalizeDraft(in,now);if err!=nil{return liveevents.Event{},err}
 if len(idem)<16||len(idem)>160{return liveevents.Event{},liveevents.ErrInvalid}
 encoded,_:=json.Marshal(struct{Org string;Draft liveevents.DraftInput}{org,in})
 sum:=sha256.Sum256(encoded);fingerprint:=hex.EncodeToString(sum[:])
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return liveevents.Event{},err}
 defer tx.Rollback()
 // Serialize equal idempotency keys even on simultaneous API processes.
 if _,err=tx.ExecContext(ctx,`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,actor+":"+idem);err!=nil{return liveevents.Event{},err}
 var existingID,existingFingerprint string
 err=tx.QueryRowContext(ctx,`SELECT event_id::text,fingerprint FROM live_event_mutations
 WHERE actor_id=$1::uuid AND idempotency_key=$2`,actor,idem).Scan(&existingID,&existingFingerprint)
 if err==nil {
  if fingerprint!=existingFingerprint{return liveevents.Event{},liveevents.ErrConflict}
  if err=tx.Commit();err!=nil{return liveevents.Event{},err}
  return c.LiveEvent(ctx,existingID,actor)
 }
 if !errors.Is(err,sql.ErrNoRows){return liveevents.Event{},err}
 var allowed bool
 if err=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM organization_ownerships o
 JOIN organizations g ON g.id=o.organization_id AND g.status='ACTIVE'
 WHERE o.organization_id=$1::uuid AND o.identity_id=$2::uuid AND o.status='ACTIVE')`,org,actor).Scan(&allowed);err!=nil{return liveevents.Event{},err}
 if !allowed{return liveevents.Event{},liveevents.ErrForbidden}
 id,err:=persistentid.New();if err!=nil{return liveevents.Event{},err}
 _,err=tx.ExecContext(ctx,`INSERT INTO live_events(id,organization_id,organizer_identity_id,title,description,
 format,starts_at,ends_at,capacity,state,price_minor,currency)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,'DRAFT',0,'RUB')`,
 id,org,actor,in.Title,in.Description,in.Format,in.StartsAt,in.EndsAt,in.Capacity)
 if err!=nil{return liveevents.Event{},fmt.Errorf("insert live event: %w",err)}
 if _,err=tx.ExecContext(ctx,`INSERT INTO live_event_mutations(actor_id,idempotency_key,action,event_id,fingerprint)
 VALUES($1::uuid,$2,'CREATE',$3::uuid,$4)`,actor,idem,id,fingerprint);err!=nil{return liveevents.Event{},err}
 if err=appendLiveEventOutbox(ctx,tx,id,org,actor,"live_event.created",now);err!=nil{return liveevents.Event{},err}
 if err=tx.Commit();err!=nil{return liveevents.Event{},err}
 return c.LiveEvent(ctx,id,actor)
}
func appendLiveEventOutbox(ctx context.Context,tx *sql.Tx,id,org,actor,kind string,at time.Time) error {
 evID,err:=persistentid.New();if err!=nil{return err}
 payload,_:=json.Marshal(map[string]string{"live_event_id":id,"organization_id":org,"state_event":kind})
 return ensureOutboxEventTx(ctx,tx,eventspine.EventEnvelope{
  EventID:evID,IdempotencyKey:kind+":"+id+":"+evID,EventType:kind,SchemaVersion:"1",
  AggregateRef:"live_event/"+id,AggregateVersion:1,OccurredAt:at.UTC(),
  Producer:"apgic-live-events",TenantScope:"organization/"+org,CorrelationID:evID,
  CausationID:actor,PayloadJSON:payload,
 })
}
func (c *Checker) ChangeLiveEvent(ctx context.Context,actor,eventID string,target liveevents.State,now time.Time)(liveevents.Event,error){
 if c==nil||c.db==nil{return liveevents.Event{},liveevents.ErrUnavailable}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return liveevents.Event{},err};defer tx.Rollback()
 var old liveevents.State
 var org string
 var start,end time.Time
 var provider sql.NullString
 var proof sql.NullTime
 var eligible bool
 err=tx.QueryRowContext(ctx,`SELECT e.state,e.organization_id::text,e.starts_at,e.ends_at,
 e.provider_instance_id::text,e.provider_proof_checked_at,
 EXISTS(SELECT 1 FROM organization_ownerships o WHERE o.organization_id=e.organization_id
 AND o.identity_id=$2::uuid AND o.status='ACTIVE')
 FROM live_events e WHERE e.id=$1::uuid FOR UPDATE OF e`,eventID,actor).
 Scan(&old,&org,&start,&end,&provider,&proof,&eligible)
 if errors.Is(err,sql.ErrNoRows){return liveevents.Event{},liveevents.ErrNotFound}
 if err!=nil{return liveevents.Event{},err}
 if !eligible{return liveevents.Event{},liveevents.ErrForbidden}
 if !liveevents.CanTransition(old,target){return liveevents.Event{},liveevents.ErrConflict}
 if target==liveevents.Published||target==liveevents.Live {
  if !provider.Valid||!proof.Valid||now.Sub(proof.Time)>5*time.Minute||proof.Time.After(now.Add(time.Minute)){
   return liveevents.Event{},liveevents.ErrUnavailable
  }
  var healthy bool
  if err=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM connector_instances
 WHERE id=$1::uuid AND capability_class='COMMUNICATION_PROVIDER' AND status='ACTIVE')`,provider.String).Scan(&healthy);err!=nil{return liveevents.Event{},err}
  if !healthy{return liveevents.Event{},liveevents.ErrUnavailable}
 }
 if target==liveevents.Published && !start.After(now){return liveevents.Event{},liveevents.ErrConflict}
 if target==liveevents.Live && (now.Before(start.Add(-15*time.Minute))||now.After(end)){return liveevents.Event{},liveevents.ErrConflict}
 if target==liveevents.Completed && now.Before(start){return liveevents.Event{},liveevents.ErrConflict}
 if _,err=tx.ExecContext(ctx,`UPDATE live_events SET state=$2,updated_at=$3 WHERE id=$1::uuid`,eventID,target,now);err!=nil{return liveevents.Event{},err}
 if err=appendLiveEventOutbox(ctx,tx,eventID,org,actor,"live_event."+strings.ToLower(string(target)),now);err!=nil{return liveevents.Event{},err}
 if err=tx.Commit();err!=nil{return liveevents.Event{},err}
 return c.LiveEvent(ctx,eventID,actor)
}
func (c *Checker) RegisterLiveEvent(ctx context.Context,actor,eventID string,now time.Time)(liveevents.Registration,error) {
 if c==nil||c.db==nil{return liveevents.Registration{},liveevents.ErrUnavailable}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return liveevents.Registration{},err};defer tx.Rollback()
 var state liveevents.State
 var start time.Time
 var capacity int
 var org string
 err=tx.QueryRowContext(ctx,`SELECT state,starts_at,capacity,organization_id::text
 FROM live_events WHERE id=$1::uuid FOR UPDATE`,eventID).Scan(&state,&start,&capacity,&org)
 if errors.Is(err,sql.ErrNoRows){return liveevents.Registration{},liveevents.ErrNotFound}
 if err!=nil{return liveevents.Registration{},err}
 if !liveevents.CanRegister(state,now,start){return liveevents.Registration{},liveevents.ErrConflict}
 var out liveevents.Registration
 err=tx.QueryRowContext(ctx,`SELECT id::text,event_id::text,identity_id::text,state,registered_at
 FROM live_event_registrations WHERE event_id=$1::uuid AND identity_id=$2::uuid`,eventID,actor).
 Scan(&out.ID,&out.EventID,&out.IdentityID,&out.State,&out.RegisteredAt)
 if err==nil{
  if out.State!="REGISTERED"{return liveevents.Registration{},liveevents.ErrConflict}
  if err=tx.Commit();err!=nil{return liveevents.Registration{},err};return out,nil
 }
 if !errors.Is(err,sql.ErrNoRows){return liveevents.Registration{},err}
 var count int
 if err=tx.QueryRowContext(ctx,`SELECT count(*) FROM live_event_registrations WHERE event_id=$1::uuid AND state='REGISTERED'`,eventID).Scan(&count);err!=nil{return liveevents.Registration{},err}
 if count>=capacity{return liveevents.Registration{},liveevents.ErrFull}
 id,err:=persistentid.New();if err!=nil{return liveevents.Registration{},err}
 if _,err=tx.ExecContext(ctx,`INSERT INTO identities(id) VALUES($1::uuid) ON CONFLICT(id) DO NOTHING`,actor);err!=nil{return liveevents.Registration{},err}
 if _,err=tx.ExecContext(ctx,`INSERT INTO live_event_registrations(id,event_id,identity_id,registered_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4)`,id,eventID,actor,now);err!=nil{return liveevents.Registration{},err}
 if err=appendLiveEventOutbox(ctx,tx,eventID,org,actor,"live_event.registered",now);err!=nil{return liveevents.Registration{},err}
 if err=tx.Commit();err!=nil{return liveevents.Registration{},err}
 return liveevents.Registration{ID:id,EventID:eventID,IdentityID:actor,State:"REGISTERED",RegisteredAt:now},nil
}
func (c *Checker) MyLiveRegistration(ctx context.Context,actor,eventID string)(*liveevents.Registration,error){
 if c==nil||c.db==nil{return nil,liveevents.ErrUnavailable}
 var out liveevents.Registration
 err:=c.db.QueryRowContext(ctx,`SELECT r.id::text,r.event_id::text,r.identity_id::text,r.state,r.registered_at
 FROM live_event_registrations r JOIN live_events e ON e.id=r.event_id
 WHERE r.event_id=$1::uuid AND r.identity_id=$2::uuid
 AND (e.state IN ('PUBLISHED','LIVE','COMPLETED') OR e.organizer_identity_id=$2::uuid)`,eventID,actor).
 Scan(&out.ID,&out.EventID,&out.IdentityID,&out.State,&out.RegisteredAt)
 if errors.Is(err,sql.ErrNoRows){return nil,nil};if err!=nil{return nil,err};return &out,nil
}
func (c *Checker) CancelLiveRegistration(ctx context.Context,actor,eventID string,now time.Time)(liveevents.Registration,error){
 if c==nil||c.db==nil{return liveevents.Registration{},liveevents.ErrUnavailable}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return liveevents.Registration{},err};defer tx.Rollback()
 var out liveevents.Registration
 var org string
 err=tx.QueryRowContext(ctx,`SELECT r.id::text,r.event_id::text,r.identity_id::text,r.state,r.registered_at,e.organization_id::text
 FROM live_event_registrations r JOIN live_events e ON e.id=r.event_id
 WHERE r.event_id=$1::uuid AND r.identity_id=$2::uuid AND e.starts_at>$3
 FOR UPDATE OF r`,eventID,actor,now).Scan(&out.ID,&out.EventID,&out.IdentityID,&out.State,&out.RegisteredAt,&org)
 if errors.Is(err,sql.ErrNoRows){return liveevents.Registration{},liveevents.ErrNotFound}
 if err!=nil{return liveevents.Registration{},err}
 if out.State=="CANCELLED"{if err=tx.Commit();err!=nil{return out,err};return out,nil}
 if _,err=tx.ExecContext(ctx,`UPDATE live_event_registrations SET state='CANCELLED',cancelled_at=$2
 WHERE id=$1::uuid`,out.ID,now);err!=nil{return liveevents.Registration{},err}
 if err=appendLiveEventOutbox(ctx,tx,eventID,org,actor,"live_event.unregistered",now);err!=nil{return liveevents.Registration{},err}
 if err=tx.Commit();err!=nil{return liveevents.Registration{},err}
 out.State="CANCELLED";return out,nil
}
func (c *Checker) LiveEventAccess(ctx context.Context,actor,eventID string,now time.Time)(liveevents.Access,error){
 if c==nil||c.db==nil{return liveevents.Access{},liveevents.ErrUnavailable}
 var state liveevents.State
 var start,end time.Time
 var reg bool
 var proof sql.NullTime
 var ready bool
 err:=c.db.QueryRowContext(ctx,`SELECT e.state,e.starts_at,e.ends_at,
 EXISTS(SELECT 1 FROM live_event_registrations r WHERE r.event_id=e.id AND r.identity_id=$2::uuid AND r.state='REGISTERED'),
 e.provider_proof_checked_at,
 EXISTS(SELECT 1 FROM connector_instances p WHERE p.id=e.provider_instance_id
 AND p.capability_class='COMMUNICATION_PROVIDER' AND p.status='ACTIVE')
 FROM live_events e WHERE e.id=$1::uuid AND e.state IN ('PUBLISHED','LIVE','COMPLETED')`,eventID,actor).
 Scan(&state,&start,&end,&reg,&proof,&ready)
 if errors.Is(err,sql.ErrNoRows){return liveevents.Access{},liveevents.ErrNotFound}
 if err!=nil{return liveevents.Access{},err}
 ready=ready&&proof.Valid&&now.Sub(proof.Time)<=5*time.Minute&&!proof.Time.After(now.Add(time.Minute))
 return liveevents.CanJoin(state,reg,now,start,end,ready),nil
}

// Signed provider callback records capability and attendance evidence; browser
// requests cannot self-attest JOINED or SFU readiness.
type LiveProviderFact struct {
 EventID string `json:"event_id"`
 FactType string `json:"fact_type"`
 RegistrationID string `json:"registration_id,omitempty"`
 ProviderEventID string `json:"provider_event_id,omitempty"`
 SFUReady bool `json:"sfu_ready,omitempty"`
}
func (c *Checker) ApplyLiveProviderWebhook(ctx context.Context,envelope connector.WebhookEnvelope,keys connector.WebhookPublicKeyResolver)(connector.DeliveryDecision,error){
 var fact LiveProviderFact
 if err:=json.Unmarshal(envelope.Payload,&fact);err!=nil{return "",liveevents.ErrInvalid}
 if fact.EventID==""||envelope.StreamID!="live_event/"+fact.EventID {return "",liveevents.ErrInvalid}
 if fact.FactType!="CAPABILITIES"&&fact.FactType!="JOINED"&&fact.FactType!="LEFT"{return "",liveevents.ErrInvalid}
 if fact.FactType!="CAPABILITIES"&&(fact.RegistrationID==""||fact.ProviderEventID==""){return "",liveevents.ErrInvalid}
 return c.ProcessConnectorWebhook(ctx,envelope,keys,func(ctx context.Context,tx *sql.Tx,_ json.RawMessage)error{
  var state liveevents.State
  var provider sql.NullString
  if err:=tx.QueryRowContext(ctx,`SELECT state,provider_instance_id::text FROM live_events WHERE id=$1::uuid FOR UPDATE`,fact.EventID).Scan(&state,&provider);err!=nil{return err}
  var communication bool
  if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM connector_instances
 WHERE id=$1::uuid AND capability_class='COMMUNICATION_PROVIDER' AND status='ACTIVE')`,
 envelope.ConnectorInstanceID).Scan(&communication);err!=nil{return err}
  if !communication{return liveevents.ErrUnavailable}
  if fact.FactType=="CAPABILITIES"{
   if state!=liveevents.Draft&&state!=liveevents.Published {return liveevents.ErrConflict}
   if provider.Valid&&provider.String!=envelope.ConnectorInstanceID{return liveevents.ErrForbidden}
   if !fact.SFUReady{return liveevents.ErrUnavailable}
   _,err:=tx.ExecContext(ctx,`UPDATE live_events SET provider_instance_id=$2::uuid,
 provider_proof_checked_at=$3,updated_at=$3 WHERE id=$1::uuid`,fact.EventID,envelope.ConnectorInstanceID,envelope.OccurredAt.UTC())
   return err
  }
  if !provider.Valid||provider.String!=envelope.ConnectorInstanceID{return liveevents.ErrForbidden}
  if state!=liveevents.Live && state!=liveevents.Published{return liveevents.ErrConflict}
  id,err:=persistentid.New();if err!=nil{return err}
  result,err:=tx.ExecContext(ctx,`INSERT INTO live_event_attendance_facts(id,event_id,registration_id,
 connector_instance_id,provider_event_id,fact_type,occurred_at)
 SELECT $1::uuid,e.id,r.id,$4::uuid,$5,$6,$7
 FROM live_events e JOIN live_event_registrations r ON r.event_id=e.id
 WHERE e.id=$2::uuid AND r.id=$3::uuid AND r.state='REGISTERED'`,
 id,fact.EventID,fact.RegistrationID,envelope.ConnectorInstanceID,fact.ProviderEventID,fact.FactType,envelope.OccurredAt.UTC())
  if err!=nil{return err}
  n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return liveevents.ErrForbidden}
  return nil
 })
}
