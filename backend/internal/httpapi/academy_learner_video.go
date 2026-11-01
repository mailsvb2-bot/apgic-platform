package httpapi

import (
 "net/http"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)
type academyLearnerVideoStore interface {
 AcademyLearnerVideoManifest(actor,enrollmentID,lessonID string)([]academy.LearnerVideo,error)
}
func registerAcademyLearnerVideoManifest(mux *http.ServeMux,store academyStore,sessions *clientSessionManager,sessionErr error){
 mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/videos",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr)
  if !ok{return}
  reader,ok:=store.(academyLearnerVideoStore)
  if !ok {academyHTTPError(w,r,academy.ErrUnavailable);return}
  videos,err:=reader.AcademyLearnerVideoManifest(actor,r.PathValue("enrollmentID"),r.PathValue("lessonID"))
  if err!=nil{academyHTTPError(w,r,err);return}
  w.Header().Set("Cache-Control","private, no-store")
  writeJSON(w,http.StatusOK,map[string]any{"videos":videos})
 })
}
