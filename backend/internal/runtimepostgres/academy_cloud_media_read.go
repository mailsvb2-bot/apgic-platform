package runtimepostgres

import (
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type AcademyLessonMediaView struct {
 LessonID string `json:"lesson_id"`
 MediaID string `json:"media_id"`
 Ordinal int `json:"ordinal"`
 Title string `json:"title"`
 Provider string `json:"provider"`
 ProviderFileRef string `json:"provider_file_ref"`
 SourceURL string `json:"source_url,omitempty"`
}

// Author-only projection: learner APIs never invoke this method.
func(c *Checker) AcademyAuthorLessonMedia(actor,courseID,versionID string)([]AcademyLessonMediaView,error){
 if !academyMediaUUID(actor)||!academyMediaUUID(courseID)||!academyMediaUUID(versionID){return nil,academy.ErrInvalid}
 if c==nil||c.db==nil{return nil,academy.ErrUnavailable}
 ctx,cancel:=academyContext();defer cancel()
 rows,err:=c.db.QueryContext(ctx,`SELECT l.id::text,media.id::text,lm.ordinal,media.title,
 media.provider,media.provider_file_ref,COALESCE(media.source_url,'')
 FROM academy_courses c
 JOIN academy_course_versions v ON v.course_id=c.id
 JOIN academy_modules m ON m.version_id=v.id
 JOIN academy_lessons l ON l.module_id=m.id
 JOIN academy_lesson_media lm ON lm.lesson_id=l.id
 JOIN academy_cloud_media media ON media.id=lm.media_id AND media.author_id=c.author_id
 WHERE c.author_id=$1::uuid AND c.id=$2::uuid AND v.id=$3::uuid
 ORDER BY m.ordinal,l.ordinal,lm.ordinal`,actor,courseID,versionID)
 if err!=nil{return nil,err};defer rows.Close()
 result:=[]AcademyLessonMediaView{}
 for rows.Next(){
  var item AcademyLessonMediaView
  if err=rows.Scan(&item.LessonID,&item.MediaID,&item.Ordinal,&item.Title,
    &item.Provider,&item.ProviderFileRef,&item.SourceURL);err!=nil{return nil,err}
  result=append(result,item)
 }
 return result,rows.Err()
}
