package httpapi

import (
 "net/http"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)
type academyCloudMediaStore interface {
 AcademyImportCloudMedia(string,[]runtimepostgres.AcademyCloudMedia)([]runtimepostgres.AcademyCloudMedia,error)
 AcademyAuthorCloudMedia(string)([]runtimepostgres.AcademyCloudMedia,error)
 AcademyAssignCloudMedia(string,string,string,[]runtimepostgres.AcademyMediaAssignment)error
 AcademyAuthorLessonMedia(string,string,string)([]runtimepostgres.AcademyLessonMediaView,error)
}
func registerAcademyCloudMedia(mux *http.ServeMux,store academyStore,sessions *clientSessionManager,sessionErr error){
 mux.HandleFunc("GET /v1/academy/author/cloud-media",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr);if !ok{return}
  owner,ok:=store.(academyCloudMediaStore);if !ok{http.Error(w,"Unavailable",503);return}
  out,err:=owner.AcademyAuthorCloudMedia(actor);if err!=nil{http.Error(w,"Unavailable",503);return}
  w.Header().Set("Cache-Control","private, no-store");writeJSON(w,200,map[string]any{"media":out})
 })
 mux.HandleFunc("POST /v1/academy/author/cloud-media/import",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr);if !ok{return}
  owner,ok:=store.(academyCloudMediaStore);if !ok{http.Error(w,"Unavailable",503);return}
  var body struct{Items []runtimepostgres.AcademyCloudMedia `json:"items"`}
  if !academyRead(w,r,&body){return}
  out,err:=owner.AcademyImportCloudMedia(actor,body.Items)
  if err!=nil{http.Error(w,"Import failed",400);return}
  w.Header().Set("Cache-Control","private, no-store");writeJSON(w,201,map[string]any{"media":out})
 })
 mux.HandleFunc("GET /v1/academy/author/courses/{courseID}/versions/{versionID}/media",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr);if !ok{return}
  owner,ok:=store.(academyCloudMediaStore);if !ok{http.Error(w,"Unavailable",503);return}
  out,err:=owner.AcademyAuthorLessonMedia(actor,r.PathValue("courseID"),r.PathValue("versionID"))
  if err!=nil{http.Error(w,"Unavailable",503);return}
  w.Header().Set("Cache-Control","private, no-store");writeJSON(w,200,map[string]any{"assignments":out})
 })
 mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions/{versionID}/media/assign",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr);if !ok{return}
  owner,ok:=store.(academyCloudMediaStore);if !ok{http.Error(w,"Unavailable",503);return}
  var body struct{Items []runtimepostgres.AcademyMediaAssignment `json:"items"`}
  if !academyRead(w,r,&body){return}
  if err:=owner.AcademyAssignCloudMedia(actor,r.PathValue("courseID"),r.PathValue("versionID"),body.Items);err!=nil{http.Error(w,"Assignment rejected",409);return}
  writeJSON(w,200,map[string]any{"state":"SAVED"})
 })
}
