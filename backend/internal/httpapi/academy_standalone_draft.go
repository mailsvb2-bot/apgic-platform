package httpapi

import (
 "net/http"
 "strings"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)
type academyStandaloneDraftStore interface{
 AcademyCreateStandaloneDraft(actor,title,description string)(academy.Course,error)
}
func registerAcademyStandaloneDraft(mux *http.ServeMux,store academyStore,sessions *clientSessionManager,sessionConfigErr error){
 mux.HandleFunc("POST /v1/academy/drafts",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionConfigErr);if !ok{return}
  writer,ok:=store.(academyStandaloneDraftStore);if !ok{academyHTTPError(w,r,academy.ErrUnavailable);return}
  var input struct{Title string `json:"title"`; Description string `json:"description"`}
  if !academyRead(w,r,&input){return}
  input.Title=strings.TrimSpace(input.Title)
  if len(input.Title)<3||len(input.Title)>200{academyHTTPError(w,r,academy.ErrInvalid);return}
  course,err:=writer.AcademyCreateStandaloneDraft(actor,input.Title,input.Description)
  if err!=nil{academyHTTPError(w,r,err);return}
  w.Header().Set("Cache-Control","private, no-store")
  writeJSON(w,http.StatusCreated,course)
 })
}
