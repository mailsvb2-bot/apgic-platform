package runtimepostgres

import (
 "context"
 "encoding/json"
 "fmt"
 "time"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// QueueDueLiveReminders emits transactional NotificationProvider intents into
// the canonical outbox. QUEUED does not imply an external push/email was sent;
// provider acknowledgement and preferences remain the notification runtime's.
func (c *Checker) QueueDueLiveReminders(ctx context.Context,at time.Time,limit int)(int,error){
 if c==nil||c.db==nil{return 0,liveevents.ErrUnavailable}
 if limit<=0||limit>200{return 0,liveevents.ErrInvalid}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return 0,err}
 defer tx.Rollback()
 rows,err:=tx.QueryContext(ctx,`SELECT i.id::text,i.event_id::text,e.organization_id::text,
 r.identity_id::text,i.code,e.starts_at
 FROM live_event_reminder_intents i
 JOIN live_event_registrations r ON r.id=i.registration_id AND r.state='REGISTERED'
 AND r.registered_at=i.registration_at
 JOIN live_events e ON e.id=i.event_id AND e.state IN ('PUBLISHED','LIVE')
 WHERE i.state='PENDING' AND i.due_at<=$1 AND e.ends_at>$1
 ORDER BY i.due_at,i.id
 LIMIT $2 FOR UPDATE OF i SKIP LOCKED`,at,limit)
 if err!=nil{return 0,err}
 type pending struct{id,eventID,org,recipient,code string;startsAt time.Time}
 var events []pending
 for rows.Next(){
  var e pending
  if err=rows.Scan(&e.id,&e.eventID,&e.org,&e.recipient,&e.code,&e.startsAt);err!=nil{
   rows.Close();return 0,err
  }
  events=append(events,e)
 }
 err=rows.Err();rows.Close()
 if err!=nil{return 0,err}
 for _,e:=range events{
  id,err:=persistentid.New();if err!=nil{return 0,err}
  payload,err:=json.Marshal(map[string]string{
   "live_event_id":e.eventID,"recipient_identity_id":e.recipient,
   "reminder_code":e.code,"starts_at":e.startsAt.UTC().Format(time.RFC3339),
   "notification_policy":"TRANSACTIONAL_R9",
  })
  if err!=nil{return 0,err}
  event:=eventspine.EventEnvelope{
   EventID:id,IdempotencyKey:"live-event-reminder:"+e.id,
   EventType:"live_event.notification_due",SchemaVersion:"1",
   AggregateRef:"live_event/"+e.eventID+"/recipient/"+e.recipient,
   AggregateVersion:1,OccurredAt:at.UTC(),ProducedAt:at.UTC(),
   Producer:"apgic-live-events",TenantScope:"organization/"+e.org,
   CorrelationID:e.id,CausationID:e.recipient,PayloadJSON:payload,
  }
  if err=ensureOutboxEventTx(ctx,tx,event);err!=nil{return 0,fmt.Errorf("queue R9 reminder: %w",err)}
  if _,err=tx.ExecContext(ctx,`UPDATE live_event_reminder_intents
 SET state='QUEUED',queued_at=$2 WHERE id=$1::uuid AND state='PENDING'`,e.id,at);err!=nil{return 0,err}
 }
 if err=tx.Commit();err!=nil{return 0,err}
 return len(events),nil
}
