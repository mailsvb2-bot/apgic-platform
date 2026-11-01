package runtimepostgres

import (
 "database/sql"
 "errors"
 "fmt"
 "strings"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type AcademyCloudMedia struct {
 ID string `json:"id"`
 Provider string `json:"provider"`
 ProviderFileRef string `json:"provider_file_ref"`
 SourceURL string `json:"source_url,omitempty"`
 Title string `json:"title"`
}
type AcademyMediaAssignment struct {
 LessonID string `json:"lesson_id"`
 MediaID string `json:"media_id"`
 Ordinal int `json:"ordinal"`
}

// AcademyAssignCloudMedia atomically replaces a lesson's ordered media references.
// Neither source URLs nor provider identifiers are exposed to learner APIs.
func (c *Checker) AcademyAssignCloudMedia(actor, courseID, versionID string, items []AcademyMediaAssignment) error {
 if len(items)==0 || len(items)>2000 {return academy.ErrInvalid}
 ctx,cancel,tx,err:=academyBegin(c);defer cancel();if err!=nil{return err};defer tx.Rollback()
 owner,err:=academyAuthor(ctx,tx,actor,courseID);if err!=nil{return err};if !owner{return academy.ErrDenied}
 var state string
 err=tx.QueryRowContext(ctx,`SELECT state FROM academy_course_versions WHERE id=$1::uuid AND course_id=$2::uuid FOR UPDATE`,versionID,courseID).Scan(&state)
 if errors.Is(err,sql.ErrNoRows){return academy.ErrNotFound};if err!=nil{return err};if state!="DRAFT"{return academy.ErrState}
 used:=map[string]map[int]bool{}
 for _,item:=range items {
  if item.Ordinal<1 || item.Ordinal>2000 || !academyMediaUUID(item.LessonID)||!academyMediaUUID(item.MediaID){return academy.ErrInvalid}
  if used[item.LessonID]==nil {used[item.LessonID]=map[int]bool{}}
  if used[item.LessonID][item.Ordinal]{return academy.ErrConflict};used[item.LessonID][item.Ordinal]=true
  var exists bool
  err=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
   JOIN academy_cloud_media media ON media.id=$3::uuid AND media.author_id=$4::uuid
   WHERE l.id=$1::uuid AND m.version_id=$2::uuid AND l.kind='VIDEO')`,
   item.LessonID,versionID,item.MediaID,actor).Scan(&exists)
  if err!=nil{return err};if !exists{return academy.ErrDenied}
 }
 for lesson:=range used {
  if _,err=tx.ExecContext(ctx,`DELETE FROM academy_lesson_media WHERE lesson_id=$1::uuid`,lesson);err!=nil{return err}
 }
 for _,item:=range items{
  if _,err=tx.ExecContext(ctx,`INSERT INTO academy_lesson_media(lesson_id,media_id,ordinal)
  VALUES($1::uuid,$2::uuid,$3)`,item.LessonID,item.MediaID,item.Ordinal);err!=nil{return err}
 }
 if err=academyEvent(ctx,tx,actor,"academy/version/"+versionID,"academy.media.assigned",
  map[string]any{"version_id":versionID,"count":len(items)});err!=nil{return err}
 return tx.Commit()
}

// AcademyImportCloudMedia inserts only opaque provider references. Public URLs
// are not returned by the learner API; author access is authenticated.
func (c *Checker) AcademyImportCloudMedia(actor string, items []AcademyCloudMedia)([]AcademyCloudMedia,error){
 if !academyMediaUUID(actor)||len(items)==0||len(items)>2000{return nil,academy.ErrInvalid}
 ctx,cancel,tx,err:=academyBegin(c);defer cancel();if err!=nil{return nil,err};defer tx.Rollback()
 output:=make([]AcademyCloudMedia,0,len(items))
 for _,it:=range items{
  if it.Provider!="YANDEX_DISK"&&it.Provider!="EXTERNAL"{return nil,academy.ErrInvalid}
  it.ProviderFileRef=strings.TrimSpace(it.ProviderFileRef);it.Title=strings.TrimSpace(it.Title)
  if len(it.ProviderFileRef)<1||len(it.ProviderFileRef)>2048||len(it.Title)<1||len(it.Title)>300||len(it.SourceURL)>4096{return nil,academy.ErrInvalid}
  if it.SourceURL!=""&&!strings.HasPrefix(it.SourceURL,"https://"){return nil,academy.ErrInvalid}
  id,err:=academyID();if err!=nil{return nil,err}
  err=tx.QueryRowContext(ctx,`INSERT INTO academy_cloud_media(id,author_id,provider,provider_file_ref,source_url,title)
   VALUES($1::uuid,$2::uuid,$3,$4,NULLIF($5,''),$6)
   ON CONFLICT(author_id,provider,provider_file_ref) DO UPDATE SET title=EXCLUDED.title
   RETURNING id::text`,id,actor,it.Provider,it.ProviderFileRef,it.SourceURL,it.Title).Scan(&it.ID)
  if err!=nil{return nil,fmt.Errorf("import media: %w",err)}
  output=append(output,it)
 }
 if err=tx.Commit();err!=nil{return nil,err}
 return output,nil
}
func academyMediaUUID(s string)bool {
 if len(s)!=36{return false}
 for i,c:=range s{
  if i==8||i==13||i==18||i==23 {if c!='-'{return false};continue}
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')||(c>='A'&&c<='F')){return false}
 }
 return true
}
func(c *Checker) AcademyAuthorCloudMedia(actor string)([]AcademyCloudMedia,error){
 if !academyMediaUUID(actor){return nil,academy.ErrDenied}
 if c==nil||c.db==nil{return nil,academy.ErrUnavailable}
 ctx,cancel:=academyContext();defer cancel()
 rows,err:=c.db.QueryContext(ctx,`SELECT id::text,provider,provider_file_ref,COALESCE(source_url,''),title
 FROM academy_cloud_media WHERE author_id=$1::uuid ORDER BY created_at,id LIMIT 5000`,actor)
 if err!=nil{return nil,err};defer rows.Close()
 out:=[]AcademyCloudMedia{}
 for rows.Next(){var m AcademyCloudMedia;if err=rows.Scan(&m.ID,&m.Provider,&m.ProviderFileRef,&m.SourceURL,&m.Title);err!=nil{return nil,err};out=append(out,m)}
 return out,rows.Err()
}
