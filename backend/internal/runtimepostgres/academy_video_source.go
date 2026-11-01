package runtimepostgres

import (
 "database/sql"
 "errors"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyLearnerVideoSource resolves a cloud source on the server only after
// verifying the exact requested enrolled lesson, asset, release and entitlement.
// The returned source is a privileged server-only object, never a JSON payload.
func (c *Checker) AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string,string,string,error) {
 if c==nil||c.db==nil{return "","","",academy.ErrUnavailable}
 for _,value:=range []string{actor,enrollmentID,lessonID,mediaID}{
  if !academyMediaUUID(value){return "","","",academy.ErrInvalid}
 }
 ctx,cancel:=academyContext()
 defer cancel()
 var author,provider,path string
 err:=c.db.QueryRowContext(ctx,`SELECT c.author_id::text,media.provider,media.provider_file_ref
 FROM academy_enrollments e
 JOIN store_entitlements t ON t.id=e.entitlement_id
 JOIN academy_course_versions v ON v.id=e.version_id AND v.state='PUBLISHED'
 JOIN academy_courses c ON c.id=v.course_id AND c.state='PUBLISHED'
 JOIN academy_modules m ON m.version_id=v.id
 JOIN academy_lessons l ON l.module_id=m.id AND l.kind='VIDEO'
 JOIN academy_lesson_media lm ON lm.lesson_id=l.id
 JOIN academy_cloud_media media ON media.id=lm.media_id AND media.author_id=c.author_id
 LEFT JOIN academy_lesson_schedules sc ON sc.lesson_id=l.id
 LEFT JOIN academy_assessment_limits limits ON limits.lesson_id=l.id
 WHERE e.id=$1::uuid AND e.identity_id=$2::uuid AND l.id=$3::uuid AND media.id=$4::uuid
 AND e.state IN ('ACTIVE','COMPLETED') AND academy_entitlement_is_active(t.id)
 AND `+academyAssessmentOpenSQL, enrollmentID,actor,lessonID,mediaID).Scan(&author,&provider,&path)
 if errors.Is(err,sql.ErrNoRows){return "","","",academy.ErrEntitlement}
 if err!=nil{return "","","",err}
 if provider!="YANDEX_DISK"{return "","","",academy.ErrUnavailable}
 return author,provider,path,nil
}
