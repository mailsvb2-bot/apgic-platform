package httpapi

import (
 "net/http"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyStore interface {
 AcademyCreateCourse(string,academy.Course)(academy.Course,error)
 AcademyNewVersion(string,string)(academy.Course,error)
 AcademyAddModule(string,string,string,academy.Module)(academy.Module,error)
 AcademyAddLesson(string,string,string,string,academy.Lesson)(academy.Lesson,error)
 AcademyPublish(string,string,string)(academy.Course,error)
 AcademyCatalog()([]academy.Course,error)
 AcademyEnroll(string,string)(academy.Enrollment,error)
 AcademyLearning(string,string)([]academy.Lesson,error)
 AcademyMarkProgress(string,string,string)error
 AcademyComplete(string,string)(academy.Certificate,error)
 AcademyVerify(string)(map[string]any,error)
}
func registerAcademy(mux *http.ServeMux,store academyStore,sessions *clientSessionManager,sessionConfigErr error) {
 mux.HandleFunc("GET /v1/academy/courses",func(w http.ResponseWriter,r *http.Request){
  if store==nil {writeDemandError(w,r,503,"ACADEMY_UNAVAILABLE","Обучение недоступно.",true,nil);return}
  courses,err:=store.AcademyCatalog()
  if err!=nil {writeDemandError(w,r,503,"ACADEMY_UNAVAILABLE","Обучение недоступно.",true,nil);return}
  writeJSON(w,200,map[string]any{"courses":courses})
 })
}