package runtimepostgres

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persona"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func (c *Checker) ListPersonas(ctx context.Context)([]persona.CatalogItem,error){
 if c==nil||c.db==nil{return nil,persona.ErrUnavailable}
 rows,err:=c.db.QueryContext(ctx,`SELECT id::text,label,language,voice_profile,visual_profile,
 supports_video,supports_voice,supports_text FROM persona_catalog WHERE state='PUBLISHED'
 ORDER BY label,id LIMIT 100`)
 if err!=nil{return nil,err};defer rows.Close()
 items:=[]persona.CatalogItem{}
 for rows.Next(){
  var p persona.CatalogItem
  if err:=rows.Scan(&p.ID,&p.Label,&p.Language,&p.VoiceProfile,&p.VisualProfile,
  &p.SupportsVideo,&p.SupportsVoice,&p.SupportsText);err!=nil{return nil,err}
  items=append(items,p)
 }
 return items,rows.Err()
}
func (c *Checker) BeginAvatar(ctx context.Context,userID,providerID,idempotencyKey,policyVersion string,s persona.Start)(persona.Session,error){
 if c==nil||c.db==nil{return persona.Session{},persona.ErrUnavailable}
 if err:=s.Validate();err!=nil{return persona.Session{},err}
 if strings.TrimSpace(userID)==""||strings.TrimSpace(providerID)==""||
 len(idempotencyKey)<8||len(idempotencyKey)>128||strings.TrimSpace(policyVersion)==""{
  return persona.Session{},persona.ErrInvalidRequest
 }
 id,err:=persistentid.New();if err!=nil{return persona.Session{},err}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return persona.Session{},err}
 defer tx.Rollback()
 // The APGIC browser cookie owns identity; an arbitrary request body does not.
 if _,err:=tx.ExecContext(ctx,`INSERT INTO identities(id) VALUES ($1::uuid) ON CONFLICT DO NOTHING`,userID);err!=nil{return persona.Session{},err}
 var catalog persona.CatalogItem
 err=tx.QueryRowContext(ctx,`SELECT id::text,label,language,voice_profile,visual_profile,
 supports_video,supports_voice,supports_text FROM persona_catalog WHERE id=$1::uuid AND state='PUBLISHED'`,s.PersonaID).
 Scan(&catalog.ID,&catalog.Label,&catalog.Language,&catalog.VoiceProfile,&catalog.VisualProfile,
 &catalog.SupportsVideo,&catalog.SupportsVoice,&catalog.SupportsText)
 if errors.Is(err,sql.ErrNoRows){return persona.Session{},persona.ErrNotFound}
 if err!=nil{return persona.Session{},err}
 if catalog.Language!=s.Language||(s.Mode=="VIDEO"&&!catalog.SupportsVideo)||
 (s.Mode=="VOICE"&&!catalog.SupportsVoice)||(s.Mode=="TEXT"&&!catalog.SupportsText){return persona.Session{},persona.ErrInvalidRequest}
 var active bool
 if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM connector_instances WHERE id=$1::uuid
 AND capability_class='PERSONA_PROVIDER' AND status='ACTIVE')`,providerID).Scan(&active);err!=nil{return persona.Session{},err}
 if !active{return persona.Session{},persona.ErrUnavailable}
 var inserted string
 err=tx.QueryRowContext(ctx,`INSERT INTO avatar_sessions
 (id,user_id,persona_id,provider_instance_id,idempotency_key,session_purpose,privacy_mode,
 mode,language,voice_profile,visual_profile,state,safety_state,policy_version)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10,$11,'PENDING','NORMAL',$12)
 ON CONFLICT (user_id,idempotency_key) DO NOTHING RETURNING id::text`,
 id,userID,s.PersonaID,providerID,idempotencyKey,s.Purpose,s.PrivacyMode,s.Mode,s.Language,
 catalog.VoiceProfile,catalog.VisualProfile,policyVersion).Scan(&inserted)
 if errors.Is(err,sql.ErrNoRows){return persona.Session{},persona.ErrIdempotency}
 if err!=nil{return persona.Session{},err}
 if err:=insertAvatarFact(ctx,tx,id,userID,"CREATED","CLIENT_REQUEST",policyVersion);err!=nil{return persona.Session{},err}
 if err:=tx.Commit();err!=nil{return persona.Session{},err}
 return persona.Session{ID:id,UserID:userID,PersonaID:s.PersonaID,ProviderInstanceID:providerID,
 Purpose:s.Purpose,PrivacyMode:s.PrivacyMode,Mode:s.Mode,Language:s.Language,
 VoiceProfile:catalog.VoiceProfile,VisualProfile:catalog.VisualProfile,State:persona.StatePending,
 SafetyState:"NORMAL",PolicyVersion:policyVersion},nil
}
func insertAvatarFact(ctx context.Context,tx *sql.Tx,id,actor,event,reason,policy string)error{
 factID,err:=persistentid.New();if err!=nil{return err}
 _,err=tx.ExecContext(ctx,`INSERT INTO avatar_session_facts
 (id,session_id,actor_id,event_type,reason_code,policy_version)
 VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,$4,$5,$6)`,factID,id,actor,event,reason,policy)
 return err
}
func(c *Checker) personaChange(ctx context.Context, id,owner,event,reason,reference,mode string)(error){
 if c==nil||c.db==nil{return persona.ErrUnavailable}
 tx,err:=c.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 var policy string
 switch event {
 case "ACTIVATED":
  err=tx.QueryRowContext(ctx,`UPDATE avatar_sessions SET state='ACTIVE',started_at=now(),mode=$3,
 provider_session_ref=$4 WHERE id=$1::uuid AND user_id=$2::uuid AND state='PENDING'
 RETURNING policy_version`,id,owner,mode,reference).Scan(&policy)
 case "FAILED":
  err=tx.QueryRowContext(ctx,`UPDATE avatar_sessions SET state='FAILED' WHERE id=$1::uuid
 AND user_id=$2::uuid AND state='PENDING' RETURNING policy_version`,id,owner).Scan(&policy)
 case "CLOSED":
  err=tx.QueryRowContext(ctx,`UPDATE avatar_sessions SET state='ENDED',ended_at=now()
 WHERE id=$1::uuid AND user_id=$2::uuid AND state='ACTIVE'
 RETURNING policy_version`,id,owner).Scan(&policy)
 case "HANDOFF_REQUESTED":
  err=tx.QueryRowContext(ctx,`UPDATE avatar_sessions SET handoff_requested_at=now()
 WHERE id=$1::uuid AND user_id=$2::uuid AND state IN ('ACTIVE','ENDED')
 AND handoff_requested_at IS NULL RETURNING policy_version`,id,owner).Scan(&policy)
 case "SAFETY_ESCALATED":
  err=tx.QueryRowContext(ctx,`UPDATE avatar_sessions SET safety_state='ESCALATED'
 WHERE id=$1::uuid AND user_id=$2::uuid AND state IN ('ACTIVE','ENDED')
 AND safety_state='NORMAL' RETURNING policy_version`,id,owner).Scan(&policy)
 default:return persona.ErrInvalidRequest
 }
 if errors.Is(err,sql.ErrNoRows){return persona.ErrConflict}
 if err!=nil{return err}
 if err:=insertAvatarFact(ctx,tx,id,owner,event,reason,policy);err!=nil{return err}
 return tx.Commit()
}
func(c *Checker) ActivateAvatar(ctx context.Context,id,owner,reference,mode string)error{
 if strings.TrimSpace(reference)==""||!(mode=="VIDEO"||mode=="VOICE"||mode=="TEXT"){return persona.ErrInvalidRequest}
 return c.personaChange(ctx,id,owner,"ACTIVATED","PROVIDER_OPEN_CONFIRMED",reference,mode)
}
func(c *Checker) FailAvatar(ctx context.Context,id,owner string)error{
 return c.personaChange(ctx,id,owner,"FAILED",persona.ReasonProviderUnavailable,"","")
}
func(c *Checker) EndAvatar(ctx context.Context,id,owner string)error{
 return c.personaChange(ctx,id,owner,"CLOSED","CLIENT_ENDED","","")
}
func(c *Checker) HandoffAvatar(ctx context.Context,id,owner string)error{
 return c.personaChange(ctx,id,owner,"HANDOFF_REQUESTED","CLIENT_REQUESTED_HUMAN","","")
}
func(c *Checker) EscalateAvatar(ctx context.Context,id,owner string)error{
 return c.personaChange(ctx,id,owner,"SAFETY_ESCALATED","CLIENT_REQUESTED_SAFETY","","")
}
func(c *Checker) ReadAvatar(ctx context.Context,id,owner string)(persona.Session,error){
 if c==nil||c.db==nil{return persona.Session{},persona.ErrUnavailable}
 var s persona.Session
 var started,ended,handoff,waitlist sql.NullTime
 var ref sql.NullString
 err:=c.db.QueryRowContext(ctx,`SELECT id::text,user_id::text,persona_id::text,provider_instance_id::text,
 provider_session_ref,session_purpose,privacy_mode,mode,language,voice_profile,visual_profile,state,safety_state,
 created_at,started_at,ended_at,handoff_requested_at,waitlist_requested_at,policy_version
 FROM avatar_sessions WHERE id=$1::uuid AND user_id=$2::uuid`,id,owner).
 Scan(&s.ID,&s.UserID,&s.PersonaID,&s.ProviderInstanceID,&ref,&s.Purpose,&s.PrivacyMode,
 &s.Mode,&s.Language,&s.VoiceProfile,&s.VisualProfile,&s.State,&s.SafetyState,
 &s.CreatedAt,&started,&ended,&handoff,&waitlist,&s.PolicyVersion)
 if errors.Is(err,sql.ErrNoRows){return persona.Session{},persona.ErrNotFound}
 if err!=nil{return persona.Session{},fmt.Errorf("read avatar: %w",err)}
 if ref.Valid{s.ProviderSessionRef=ref.String}
 if started.Valid{t:=started.Time;s.StartedAt=&t}
 if ended.Valid{t:=ended.Time;s.EndedAt=&t}
 if handoff.Valid{t:=handoff.Time;s.HandoffRequestedAt=&t}
 if waitlist.Valid{t:=waitlist.Time;s.WaitlistRequestedAt=&t}
 return s,nil
}
