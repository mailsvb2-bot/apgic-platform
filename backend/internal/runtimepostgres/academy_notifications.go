package runtimepostgres

import (
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func (c *Checker) AcademyNotificationPreferences(actor string) (academy.NotificationPreferences,error) {
 if c==nil||c.db==nil{return academy.NotificationPreferences{},academy.ErrUnavailable}
 ctx,cancel:=academyContext();defer cancel()
 var p academy.NotificationPreferences
 err:=c.db.QueryRowContext(ctx,`SELECT enabled,learning_updates,assessment_results,certificate_updates
 FROM academy_notification_preferences WHERE identity_id=$1::uuid`,actor).
 Scan(&p.Enabled,&p.LearningUpdates,&p.AssessmentResults,&p.CertificateUpdates)
 if err!=nil {
  // No preferences means opt-out, never an implicit opt-in.
  if err.Error()=="sql: no rows in result set"{return academy.NotificationPreferences{},nil}
  return academy.NotificationPreferences{},err
 }
 return p,nil
}

func (c *Checker) AcademySaveNotificationPreferences(actor string,p academy.NotificationPreferences) (academy.NotificationPreferences,error) {
 if c==nil||c.db==nil{return academy.NotificationPreferences{},academy.ErrUnavailable}
 if !p.Enabled {p.LearningUpdates=false;p.AssessmentResults=false;p.CertificateUpdates=false}
 ctx,cancel,tx,err:=academyBegin(c);defer cancel()
 if err!=nil{return academy.NotificationPreferences{},err}
 defer tx.Rollback()
 _,err=tx.ExecContext(ctx,`INSERT INTO academy_notification_preferences(identity_id,enabled,learning_updates,assessment_results,certificate_updates)
 VALUES($1::uuid,$2,$3,$4,$5) ON CONFLICT(identity_id) DO UPDATE
 SET enabled=EXCLUDED.enabled, learning_updates=EXCLUDED.learning_updates,
 assessment_results=EXCLUDED.assessment_results,certificate_updates=EXCLUDED.certificate_updates,
 updated_at=now()`,actor,p.Enabled,p.LearningUpdates,p.AssessmentResults,p.CertificateUpdates)
 if err!=nil{return academy.NotificationPreferences{},err}
 if err=academyEvent(ctx,tx,actor,"academy/notifications/"+actor,
 "academy.notification.preferences.changed",map[string]any{"enabled":p.Enabled});err!=nil{return academy.NotificationPreferences{},err}
 if err=tx.Commit();err!=nil{return academy.NotificationPreferences{},err}
 return p,nil
}
