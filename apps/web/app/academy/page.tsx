"use client";

import {useCallback,useEffect,useMemo,useRef,useState,type FormEvent} from "react";
import CloudMediaEditor from "./cloud-media-editor";
import AcademyLessonVideos from "./academy-lesson-videos";
import "./academy-workspace.css";

type AcademyNotifications = {enabled:boolean;learning_updates:boolean;assessment_results:boolean;certificate_updates:boolean};
type Course = {id:string;product_id:string;title:string;description:string;state:string;version_id:string;version_no:number;access_mode?:"FREE"|"PAID"|"UNLISTED";price_minor?:number;currency?:string};
type CourseOffer = {course_id:string;access_mode:"FREE"|"PAID";price_minor:number;currency:"RUB";revision:number};
type Module = {id:string;version_id:string;title:string;ordinal:number};
type Lesson = {id:string;module_id:string;title:string;kind:"READING"|"VIDEO"|"ASSESSMENT";ordinal:number;body?:string;media_asset_ref?:string;completed?:boolean;passing_score?:number;available?:boolean};
type Enrollment = {id:string;version_id:string;course_id?:string;course_title?:string;version_no?:number;state:string;access_state?:"ACTIVE"|"REVOKED"};
type Certificate = {verification_url:string;state:string;evidence_candidate_state:string};
type LearnerCertificate = {id:string;enrollment_id:string;course_title:string;course_version:number;state:string;learning_access_state:string;verification_url:string;evidence_candidate_state:string;issued_at:string};
type Cohort = {id:string;version_id:string;label:string;capacity:number;members:number;is_member?:boolean;starts_at?:string;enrollment_closes_at?:string};
type LearnerAppeal = {id:string;attempt_id:string;state:string;reason:string;submitted_at:string};
type LearnerAttempt = {id:string;lesson_id:string;attempt_no:number;result:"PENDING"|"PASSED"|"FAILED";score?:number;submitted_at:string;evaluated_at?:string};
type ReviewAttempt = {attempt_id:string;enrollment_id:string;lesson_id:string;attempt_no:number;response_text:string;submitted_at:string;rubric_version:string;passing_score:number;course_title:string;lesson_title:string};
type AuthorReport = {courses:{course_id:string;title:string;enrollments:number;completed:number;active_access:number;pending_assessments:number;passed_assessments:number}[];gross_captures:{course_id:string;currency:string;captured_count:number;captured_gross_minor:number}[];revenue_state:string};

async function api<T>(path:string,body?:unknown):Promise<T>{
 const response=await fetch(path,{method:body===undefined?"GET":"POST",
  headers:{"Content-Type":"application/json"},
  credentials:"same-origin",cache:"no-store",
  body:body===undefined?undefined:JSON.stringify(body)});
 const data=await response.json().catch(()=>({}));
 if(!response.ok)throw Object.assign(new Error(data.message_safe||data.code||"Сервис временно недоступен"),{code:data.code,status:response.status});
 return data as T;
}

function draftWriteWasRejected(error:unknown):boolean{
 // Only definitive application-level rejections prove that no draft was
 // committed. A timeout or 5xx can happen after commit: reconcile those.
 const status=(error as {status?:number}|null)?.status;
 return status===400||status===401||status===403||status===404||status===409||status===422;
}

async function downloadAcademyHistory():Promise<void>{
 const response=await fetch("/v1/academy/me/history/export",{
  method:"GET",credentials:"same-origin",cache:"no-store",headers:{"Accept":"application/json"}
 });
 if(!response.ok){
  const problem=await response.json().catch(()=>({}));
  throw new Error(problem.message_safe||problem.code||"Экспорт недоступен");
 }
 const blob=await response.blob();
 const url=URL.createObjectURL(blob);
 try{
  const link=document.createElement("a");
  link.href=url;link.download="apgic-academy-history.json";
  document.body.appendChild(link);link.click();link.remove();
 }finally{URL.revokeObjectURL(url)}
}

export default function AcademyPage(){
 const [view,setView]=useState<"home"|"courses"|"learning"|"catalog"|"reports"|"reviews">("home");
 const [editorStep,setEditorStep]=useState<"structure"|"media"|"publish"|"preview">("structure");
 const [showCreate,setShowCreate]=useState(false);
 const [catalog,setCatalog]=useState<Course[]>([]);
 const [catalogError,setCatalogError]=useState("");
 const [mine,setMine]=useState<Course[]>([]);
 const [ownedCoursesError,setOwnedCoursesError]=useState("");
 const [ownedCoursesLoaded,setOwnedCoursesLoaded]=useState(false);
 const [authorSessionMissing,setAuthorSessionMissing]=useState(false);
 const [enrollments,setEnrollments]=useState<Enrollment[]>([]);
 const [learningError,setLearningError]=useState("");
 const [verifiedCourseIDs,setVerifiedCourseIDs]=useState<string[]>([]);
 const [course,setCourse]=useState<Course|null>(null);
 const [openedDeepLink,setOpenedDeepLink]=useState(false);
 const [modules,setModules]=useState<Module[]>([]);
 const [lessons,setLessons]=useState<Lesson[]>([]);
 // Stable options prevent a parent redraw from resetting the author’s file
 // selection while the selected lesson is unchanged.
 const mediaLessonOptions=useMemo(()=>lessons.map(l=>({id:l.id,title:l.title,kind:l.kind,module_id:l.module_id})),[lessons]);
 const mediaModuleOptions=useMemo(()=>modules.map(m=>({id:m.id,title:m.title,ordinal:m.ordinal})),[modules]);
 const [focusedVideoLessonID,setFocusedVideoLessonID]=useState("");
 const [previewLessonID,setPreviewLessonID]=useState("");
 const [activeEnrollment,setActiveEnrollment]=useState<Enrollment|null>(null);
 const [certificate,setCertificate]=useState<Certificate|null>(null);
 const [certificates,setCertificates]=useState<LearnerCertificate[]>([]);
 const [certificatesUnavailable,setCertificatesUnavailable]=useState(false);
 const [cohorts,setCohorts]=useState<Cohort[]>([]);
 const [cohortError,setCohortError]=useState(false);
 const [appealError,setAppealError]=useState(false);
 const [authorCohorts,setAuthorCohorts]=useState<Cohort[]>([]);
 const [cohortTitle,setCohortTitle]=useState("");
 const [cohortCapacity,setCohortCapacity]=useState(20);
 const [cohortStarts,setCohortStarts]=useState("");
 const [cohortCloses,setCohortCloses]=useState("");
 const [notice,setNotice]=useState("");
 const [busy,setBusy]=useState(false);
 // A lost HTTP response must not lead to a second, non-idempotent draft POST.
 const [pendingDraft,setPendingDraft]=useState<{title:string;description:string;requestID:string}|null>(null);
 const [confirmedDraft,setConfirmedDraft]=useState<Course|null>(null);
 const [notifications,setNotifications]=useState<AcademyNotifications|null>(null);
 const [title,setTitle]=useState("");
 const [description,setDescription]=useState("");
 const [moduleTitle,setModuleTitle]=useState("");
 const [structureAction,setStructureAction]=useState<"overview"|"module"|"lesson">("overview");
 const [lessonTitle,setLessonTitle]=useState("");
 const [lessonBody,setLessonBody]=useState("");
 const [lessonKind,setLessonKind]=useState<Lesson["kind"]>("VIDEO");
 const [moduleID,setModuleID]=useState("");
 const [passingScore,setPassingScore]=useState(80);
 const [attemptResponse,setAttemptResponse]=useState<Record<string,string>>({});
 const [learnerAttempts,setLearnerAttempts]=useState<LearnerAttempt[]>([]);
 const [attemptHistoryError,setAttemptHistoryError]=useState(false);
 const [learnerAppeals,setLearnerAppeals]=useState<LearnerAppeal[]>([]);
 const [attemptID,setAttemptID]=useState("");
 const [pendingReviews,setPendingReviews]=useState<ReviewAttempt[]>([]);
 const [reviewError,setReviewError]=useState("");
 const [authorReport,setAuthorReport]=useState<AuthorReport|null>(null);
 const [evaluationScore,setEvaluationScore]=useState(80);
 const [evaluationEvidence,setEvaluationEvidence]=useState("");
 const [rightsSource,setRightsSource]=useState("");
 const [rightsAgreed,setRightsAgreed]=useState(false);
 const [rightsRecorded,setRightsRecorded]=useState(false);
 const [publishError,setPublishError]=useState("");
 const [rightsFeedback,setRightsFeedback]=useState("");
 const [offerFeedback,setOfferFeedback]=useState<{kind:"success"|"error";message:string}|null>(null);
 const rightsInputRef=useRef<HTMLInputElement>(null);
 // The OAuth callback is consumed once per navigation; an unrelated refresh
 // must not reopen media and erase unsaved work in another editor step.
 const consumedCloudCallback=useRef<string|null>(null);
 const [offerMode,setOfferMode]=useState<"FREE"|"PAID">("FREE");
 const [offerPriceRub,setOfferPriceRub]=useState("1000");
 const [checkoutProblem,setCheckoutProblem]=useState<Record<string,string>>({});

 const refresh=useCallback(async()=>{
  // Public discovery must never become a prerequisite for author work or
  // learner history: each canonical read has an independent failure boundary.
  const [publicCourses,owned,learning,reviews,report,cohortList,access,credentials]=await Promise.allSettled([
   api<{courses:Course[]}>("/v1/academy/courses"),
   api<{courses:Course[]}>("/v1/academy/my-courses"),
   api<{enrollments:Enrollment[]}>("/v1/academy/my-enrollments"),
   api<{attempts:ReviewAttempt[]}>("/v1/academy/review/attempts"),
   api<AuthorReport>("/v1/academy/author/report"),
   api<{cohorts:Cohort[]}>("/v1/academy/author/cohorts"),
   api<{course_ids:string[]}>("/v1/academy/me/course-access"),
   api<{certificates:LearnerCertificate[]}>("/v1/academy/me/certificates")
  ]);
  setCatalog(publicCourses.status==="fulfilled"?publicCourses.value.courses:[]);
  setCatalogError(publicCourses.status==="rejected"
   ?publicCourses.reason instanceof Error?publicCourses.reason.message:"Ошибка загрузки каталога":"");
  if(owned.status==="fulfilled"){
   setMine(owned.value.courses);
   setOwnedCoursesError("");
   setAuthorSessionMissing(false);
  }else{
   setMine([]);
   // A previously open author editor must not remain visible after its
   // owner-scoped course list becomes unauthorizable or unavailable.
   setCourse(null);
   setOwnedCoursesError(owned.reason instanceof Error?owned.reason.message:"Не удалось проверить авторскую сессию");
   setAuthorSessionMissing((owned.reason as {code?:string})?.code==="CLIENT_SESSION_REQUIRED");
  }
  setOwnedCoursesLoaded(true);
  if(owned.status==="rejected"&&learning.status==="rejected"){
   setView(current=>current==="home"?"catalog":current);
  }
  if(owned.status==="fulfilled"&&owned.value.courses.length===0&&
     learning.status==="fulfilled"&&learning.value.enrollments.length>0){
   setView(current=>current==="home"?"learning":current);
  }
  // These are identity-scoped projections. An expired session or a failed
  // readback must never leave the previous owner's private history on screen.
  setEnrollments(learning.status==="fulfilled"?learning.value.enrollments:[]);
  // The learner lesson panel is a privileged projection too: revoke it when
  // the owner-scoped enrollment list can no longer be verified.
  if(learning.status==="rejected")setActiveEnrollment(null);
  setLearningError(learning.status==="rejected"
   ?learning.reason instanceof Error?learning.reason.message:"Не удалось проверить зачисления":"");
  setPendingReviews(reviews.status==="fulfilled"?reviews.value.attempts:[]);
  setReviewError(reviews.status==="rejected"
   ?reviews.reason instanceof Error?reviews.reason.message:"Не удалось проверить работы":"");
  setAuthorReport(report.status==="fulfilled"?report.value:null);
  setAuthorCohorts(cohortList.status==="fulfilled"?cohortList.value.cohorts:[]);
  // Fail closed on expired sessions or unavailable entitlement projection.
  setVerifiedCourseIDs(access.status==="fulfilled"?access.value.course_ids:[]);
  setCertificates(credentials.status==="fulfilled"?credentials.value.certificates:[]);
  setCertificatesUnavailable(credentials.status!=="fulfilled");
 },[]);
 useEffect(()=>{void refresh().catch(error=>setNotice(String(error)))},[refresh]);
 // Links from the specialist workspace select a real Academy section.
 // OAuth course/version return parameters remain owned by the existing effect.
 useEffect(()=>{
  const params=new URLSearchParams(window.location.search);
  const tab=params.get("tab");
  if(tab==="reviews"||tab==="reports"||tab==="learning"||tab==="catalog")setView(tab);
  if(tab==="courses"){setView("courses");if(params.get("create")==="1")setShowCreate(true);}
 },[]);

 async function act<T>(work:()=>Promise<T>,after:(result:T)=>Promise<void>|void){
  setBusy(true);setNotice("");
  try{const result=await work();await after(result);await refresh()}
  catch(error){setNotice(error instanceof Error?error.message:"Операция не выполнена")}
  finally{setBusy(false)}
 }
 const loadOutline=useCallback(async (selected:Course)=>{
  setPublishError("");setRightsFeedback("");setOfferFeedback(null);setRightsRecorded(false);setRightsAgreed(false);setRightsSource("");
  setOfferMode(selected.access_mode==="PAID"?"PAID":"FREE");
  setOfferPriceRub(selected.price_minor?String(selected.price_minor/100):"1000");
  setView("courses");setCourse(selected);setModules([]);setLessons([]);setPreviewLessonID("");setFocusedVideoLessonID("");setActiveEnrollment(null);setCertificate(null);setLearnerAttempts([]);setCohorts([]);
  const d=await api<{state:string;modules:Module[];lessons:Lesson[]}>(
   `/v1/academy/courses/${selected.id}/versions/${selected.version_id}/outline`);
  setModules(d.modules);setLessons(d.lessons);setModuleID(d.modules[0]?.id||"");
  if(selected.state==="DRAFT"){
   try{
    const rights=await api<{confirmed:boolean;provenance_ref:string}>(
     `/v1/academy/courses/${selected.id}/versions/${selected.version_id}/rights/original`);
    if(rights.confirmed){
     setRightsRecorded(true);setRightsSource(rights.provenance_ref);setRightsAgreed(true);
     setRightsFeedback("Права уже подтверждены для этой версии.");
    }
   }catch{
    setRightsFeedback("Не удалось проверить сохранённые права. Можно подтвердить их заново.");
   }
  }
  setStructureAction("overview");
 },[]);
 // Open a specific course from the specialist workspace without creating a duplicate.
 // The canonical my-courses response is the only source of selectable identities.
 useEffect(()=>{
  if(!ownedCoursesLoaded||openedDeepLink||ownedCoursesError)return;
  const params=new URLSearchParams(window.location.search);
  if(params.get("tab")!=="courses"||!params.get("course_id")||!params.get("version_id"))return;
  setOpenedDeepLink(true);
  const selected=mine.find(c=>c.id===params.get("course_id")&&c.version_id===params.get("version_id"));
  if(!selected){setNotice("Курс не найден в этом аккаунте. Выберите курс из списка.");return;}
  void loadOutline(selected).catch(()=>setNotice("Не удалось открыть выбранный курс. Выберите его из списка."));
 },[mine,ownedCoursesLoaded,openedDeepLink,ownedCoursesError,loadOutline]);
 // Resume a verified author's draft directly after the OAuth callback.
 // A missing course/version pair is not an error when exactly one draft exists.
 useEffect(()=>{
  const params=new URLSearchParams(window.location.search);
  const cloudError=params.get("cloud_error");
  if(params.get("cloud_connected")!=="yandex"&&cloudError!=="denied"&&cloudError!=="failed")return;
  if(!ownedCoursesLoaded||ownedCoursesError)return;
  const callbackKey=window.location.search;
  if(consumedCloudCallback.current===callbackKey)return;
  consumedCloudCallback.current=callbackKey;
  const errorNotice=cloudError==="denied"
   ? "Доступ к Яндекс Диску не предоставлен. Чтобы добавить видео, нажмите «Подключить Яндекс Диск» и подтвердите доступ."
   : cloudError==="failed"?"Яндекс не завершил подключение Диска. Повторите подключение чуть позже.":"";
  const drafts=mine.filter(c=>c.state==="DRAFT");
  const courseId=params.get("course_id"),versionId=params.get("version_id");
  const selected=courseId&&versionId
   ? drafts.find(c=>c.id===courseId&&c.version_id===versionId)
   : drafts.length===1?drafts[0]:undefined;
  if(errorNotice&&!selected){setNotice(errorNotice);return;}
  if(!selected){
   if(drafts.length>1)setNotice("Яндекс Диск подключён. Выберите нужный черновик ниже.");
   else if(drafts.length===0)setNotice("Яндекс Диск подключён. Создайте курс, чтобы добавить видео.");
   else setNotice("Яндекс Диск подключён. Выберите черновик ниже.");
   return;
  }
  let cancelled=false;
  void loadOutline(selected).then(()=>{
   if(cancelled)return;
   setEditorStep("media");setNotice(errorNotice||"Яндекс Диск подключён. Открываем ваши видео...");
   window.requestAnimationFrame(()=>{
    document.getElementById("academy-media-editor")?.scrollIntoView({behavior:"smooth",block:"start"});
   });
  }).catch(()=>{
   if(!cancelled)setNotice("Не удалось открыть редактор. Выберите курс ниже.");
  });
  return ()=>{cancelled=true};
 },[mine,loadOutline,ownedCoursesLoaded,ownedCoursesError]);
 async function loadLearning(item:Enrollment){
  const [d,historyResult,cohortResult,appealList]=await Promise.all([
   api<{lessons:Lesson[]}>(`/v1/academy/enrollments/${item.id}/learn`),
   api<{attempts:LearnerAttempt[]}>(`/v1/academy/enrollments/${item.id}/attempts`)
    .then(value=>({ok:true,items:value.attempts})).catch(()=>({ok:false,items:[] as LearnerAttempt[]})),
   api<{cohorts:Cohort[]}>(`/v1/academy/enrollments/${item.id}/cohorts`)
    .then(value=>({ok:true,items:value.cohorts})).catch(()=>({ok:false,items:[] as Cohort[]})),
   api<{appeals:LearnerAppeal[]}>("/v1/academy/me/appeals")
    .then(value=>({ok:true,items:value.appeals})).catch(()=>({ok:false,items:[] as LearnerAppeal[]}))
  ]).catch(error=>{
   // A rejected canonical /learn entitlement readback invalidates the
   // previously opened learner projection. Never keep old lesson bytes in
   // the React tree after a denied or unavailable protected read.
   setActiveEnrollment(null);
   setLessons([]);
   setLearnerAttempts([]);
   setLearnerAppeals([]);
   setCohorts([]);
   setAttemptHistoryError(false);
   setCohortError(false);
   setAppealError(false);
   throw error;
  });
  setView("learning");setActiveEnrollment(item);setCourse(null);setCertificate(null);
  setLessons(d.lessons);setLearnerAttempts(historyResult.items);setAttemptHistoryError(!historyResult.ok);
  setLearnerAppeals(appealList.items);setModules([]);
  setCohorts(cohortResult.items);setCohortError(!cohortResult.ok);setAppealError(!appealList.ok);
 }
 type DraftAttempt={title:string;description:string;requestID:string};
 async function openAcknowledgedDraft(created:Course,attempt:DraftAttempt,message:string){
  if(!created.id||!created.version_id||!created.product_id){
   setPendingDraft(attempt);
   setNotice("Сервер не вернул идентификатор созданного курса. Проверить сохранение можно безопасно тем же запросом.");
   return;
  }
  setPendingDraft(null);setConfirmedDraft(created);
  try{
   setEditorStep("structure");
   await loadOutline(created);
   await refresh();
   // refresh() isolates failures rather than throwing. Never discard the
   // acknowledged ID unless the signed owner's list confirms it.
   const readback=await api<{courses:Course[]}>("/v1/academy/my-courses");
   if(!readback.courses.some(item=>item.id===created.id&&item.version_id===created.version_id)){
    throw Error("Черновик пока не появился в подтверждённом списке автора");
   }
   setConfirmedDraft(null);
   setNotice(message);
  }catch(error){
   setCourse(null);
   setNotice("Курс уже создан и сохранён. Нажмите «Открыть созданный курс» ниже. "+String(error));
  }
 }
 async function verifyDraft(){
  if(!pendingDraft||busy)return;
  const attempt=pendingDraft;
  setBusy(true);setNotice("");
  try{
   // No title-based matching or second create path: replay one canonical
   // owner-bound request. PostgreSQL returns the original IDs atomically.
   const created=await api<Course>("/v1/academy/drafts",{
    title:attempt.title,description:attempt.description,request_id:attempt.requestID});
   await openAcknowledgedDraft(created,attempt,"Черновик найден и открыт без дублирования.");
  }catch(error){
   // The original request may already have committed; even a later 401
   // cannot prove it was not saved. Keep the owner-bound request identity.
   if((error as {status?:number})?.status===401)setAuthorSessionMissing(true);
   setNotice("Не удалось проверить сохранение. Исходный запрос сохранён без повторного создания: "+
    (error instanceof Error?error.message:String(error)));
  }finally{setBusy(false)}
 }
 async function startAuthorSession(){
  if(busy)return;
  setBusy(true);setNotice("");
  try{
   await api<{session_ready:boolean}>("/v1/academy/session/start",{});
   window.dispatchEvent(new Event("apgic:client-session-established"));
   await refresh();
   setNotice("Сессия автора создана. Теперь нажмите «Создать курс».");
  }catch(error){setNotice("Не удалось начать новую сессию: "+(error instanceof Error?error.message:String(error)))}
  finally{setBusy(false)}
 }
 async function createCourse(event:FormEvent){
  event.preventDefault();
  if(busy||pendingDraft||confirmedDraft||authorSessionMissing||title.trim().length<1)return;
  setBusy(true);setNotice("");
  const requested:DraftAttempt={title:title.trim(),description,requestID:crypto.randomUUID()};
  const body={title:requested.title,description:requested.description,request_id:requested.requestID};
  try{
   let created:Course;
   try{
    created=await api<Course>("/v1/academy/drafts",body);
   }catch(error){
    if(draftWriteWasRejected(error)){
     if((error as {status?:number}).status===401)setAuthorSessionMissing(true);
     setNotice("Курс не был создан: "+(error instanceof Error?error.message:"Сервер отклонил запрос")+
      ". Проверьте вход в аккаунт и повторите сохранение.");
     return;
    }
    // A reply might be lost after COMMIT; retry the *same* request key
    // exactly once. The server returns the existing private draft.
    try{
     created=await api<Course>("/v1/academy/drafts",body);
    }catch(retryError){
     if((retryError as {status?:number})?.status===401)setAuthorSessionMissing(true);
     // A previous ambiguous write might have committed even if this
     // readback/retry is rejected. Retain its key until proven otherwise.
     setPendingDraft(requested);
     setNotice("Ответ сервера не получен. Проверить тот же запрос без риска дубликата можно ниже.");
     return;
    }
   }
   await openAcknowledgedDraft(created,requested,"Черновик курса сохранён.");
  }finally{setBusy(false)}
 }
 async function addModule(event:FormEvent){
  event.preventDefault();if(!course)return;
  await act(()=>api<Module>(
   `/v1/academy/courses/${course.id}/versions/${course.version_id}/modules`,
   {title:moduleTitle,ordinal:modules.length+1}),async created=>{
    setModuleTitle("");
    await loadOutline(course);
    if(created.id)setModuleID(created.id);
    setStructureAction("lesson");
    setNotice("Модуль добавлен. Теперь создайте в нём первый урок.");
   });
 }
 async function addLesson(event:FormEvent){
  event.preventDefault();if(!course||!moduleID)return;
  const existing=lessons.filter(x=>x.module_id===moduleID);
  await act(()=>api<Lesson>(
   `/v1/academy/courses/${course.id}/versions/${course.version_id}/modules/${moduleID}/lessons`,
   {title:lessonTitle,ordinal:existing.length+1,kind:lessonKind,
    body:lessonKind==="READING"?lessonBody:"",
    // Video sources are attached later through the verified Disk picker;
    // never reuse hidden/stale text input as a media asset identifier.
    passing_score:lessonKind==="ASSESSMENT"?passingScore:0,
    rubric_version:lessonKind==="ASSESSMENT"?"human-review-v1":""}),
   async created=>{
    setLessonTitle("");setLessonBody("");
    await loadOutline(course);setModuleID(moduleID);setStructureAction("overview");
    if(created.kind==="VIDEO"&&created.id){
     setEditorStep("media");
     setFocusedVideoLessonID(created.id);
     window.requestAnimationFrame(()=>document.getElementById("academy-media-editor")?.scrollIntoView({behavior:"smooth",block:"start"}));
    }
   });
 }
 async function openAuthorPreview(selected:Course,lessonID?:string){
  setBusy(true);setNotice("");setEditorStep("structure");
  try{
   await loadOutline(selected);
   setPreviewLessonID(lessonID||"");
   setEditorStep("preview");
  }catch(error){
   setNotice(error instanceof Error?error.message:"Не удалось открыть предпросмотр.");
  }finally{setBusy(false)}
 }
 return <main className="academy-workspace">
  <aside className="academy-sidebar" aria-label="Навигация Академии">
   <a className="academy-logo" href="/"><span className="academy-logo-mark">A</span><span>APGIC<small>Академия</small></span></a>
   <nav aria-label="Разделы кабинета">
    {([["home","Главная","⌂"],["courses","Мои курсы","▤"],["learning","Моё обучение","▷"],["catalog","Каталог","◈"],["reviews","Проверка работ","✓"],["reports","Ученики и продажи","▥"]] as const).map(([key,label,icon])=>
     <button type="button" key={key} aria-current={view===key?"page":undefined} className={view===key?"academy-nav-current":""} onClick={()=>{setView(key);if(key==="courses")setCourse(null)}}>
      <span aria-hidden="true">{icon}</span>{label}</button>)}
    <a href="/account">⚙　Мой аккаунт</a><a href="/organization">◇　Организация</a><a href="/specialist">○　Специалист</a>
   </nav>
  </aside>
  <div className="academy-main">
   <header className="academy-header">
    <div><p>APGIC / Академия</p><strong>{view==="home"?"Кабинет автора":view==="courses"?"Мои курсы":view==="learning"?"Моё обучение":view==="catalog"?"Каталог":view==="reviews"?"Проверка работ":"Ученики и продажи"}</strong></div>
    <button className="academy-blue-button" type="button" onClick={()=>{setView("courses");setCourse(null);setShowCreate(true)}}>＋ Создать курс</button>
   </header>
   {notice&&<p role="status" aria-live="polite" className="academy-notice">{notice}</p>}
   {view==="home"&&<div className="academy-home">
    <div className="academy-intro"><p>Ваше пространство</p><h1>Кабинет автора</h1><span>Курсы, ученики и результаты — всё под рукой.</span></div>
    <section className="academy-feature">
     <div className="academy-feature-art" aria-hidden="true">▶</div>
     <div><p className="academy-overline">ПРОДОЛЖИТЬ РАБОТУ</p>
      <h2>{mine.find(c=>c.state==="DRAFT")?.title||mine[0]?.title||"Создайте первый курс"}</h2>
      <p>{mine.some(c=>c.state==="DRAFT")?"Продолжите работу с сохранённым черновиком.":mine.length?"Все курсы опубликованы. Откройте существующий курс для просмотра или создайте новый черновик.":"Сначала название, затем уроки и видео. Всё по шагам."}</p>
      <button type="button" disabled={busy||Boolean(ownedCoursesError)} className="academy-blue-button" onClick={()=>{const draft=mine.find(c=>c.state==="DRAFT");if(draft){setEditorStep("structure");void act(()=>Promise.resolve(draft),loadOutline)}else{setView("courses");setCourse(null);setShowCreate(true)}}}>{mine.some(c=>c.state==="DRAFT")?"Продолжить редактирование":"Создать новый курс"} →</button>
     </div>
    </section>
    <div className="academy-home-grid">
     <section className="academy-home-card"><header><h2>Мои курсы</h2><button type="button" onClick={()=>{setView("courses");setCourse(null)}}>Все курсы →</button></header>
      {ownedCoursesError?<p role="alert">{ownedCoursesError}</p>:mine.length===0?<p>Курсов пока нет. Создайте первый курс.</p>:
       <ul>{mine.slice(0,3).map(c=><li key={c.id}><span><strong>{c.title}</strong><small>{c.state==="PUBLISHED"?"Опубликован":"Черновик"} · версия {c.version_no}</small></span>
        <button type="button" disabled={busy} onClick={()=>{setEditorStep("structure");void act(()=>Promise.resolve(c),loadOutline)}}>Открыть →</button>
       </li>)}</ul>}
     </section>
     <section className="academy-home-card"><header><h2>Быстрые действия</h2></header>
      <div className="academy-quick-grid">
       <button type="button" onClick={()=>{setView("courses");setCourse(null)}}>▤ К списку курсов</button>
       <button type="button" disabled={busy||!mine.some(x=>x.state==="DRAFT")}
        title={mine.some(x=>x.state==="DRAFT")?"Открыть видеоуроки черновика":"Видео добавляется только в черновик. Для опубликованного курса сначала создайте новую версию."}
        onClick={()=>{const draft=mine.find(x=>x.state==="DRAFT");if(draft){void act(()=>Promise.resolve(draft),async()=>{await loadOutline(draft);setEditorStep("media")})}}}>
        {mine.some(x=>x.state==="DRAFT")?"▷ Добавить видео":"▷ Видео: нужен черновик"}</button>
       <button type="button" onClick={()=>setView("reviews")}>✓ Проверить работы</button>
       <button type="button" onClick={()=>setView("reports")}>▥ Результаты курсов</button>
       <button type="button" disabled={busy||authorSessionMissing} onClick={()=>{
        if(!window.confirm("Отключить Яндекс Диск? Все ранее подтверждённые видео станут недоступны ученикам до повторной проверки после подключения."))return;
        void act(()=>api<{disconnected:boolean}>("/v1/academy/author/cloud/yandex/disconnect",{confirm:"DISCONNECT_YANDEX_DISK"}),()=>{
         setNotice("Яндекс Диск отключён. Старые видео больше не доступны ученикам; подключите Диск заново перед использованием видео.");
        });
       }}>Отключить Яндекс Диск</button>
      </div>
     </section>
    </div>
    <div className="academy-home-grid">
     <button type="button" className="academy-metric" onClick={()=>setView("reports")}><strong>{authorReport?.courses.reduce((sum,c)=>sum+c.enrollments,0)??"—"}</strong><span>Зачислений на курсы</span><small>По данным Academy →</small></button>
     <button type="button" className="academy-metric" onClick={()=>setView("reviews")}><strong>{pendingReviews.length}</strong><span>Работ на проверку</span><small>Открыть работы →</small></button>
    </div>
   </div>}
  {(view==="catalog"||view==="learning"||(view==="courses"&&!course))&&<section className="how-section academy-main-section">
   <h1>{view==="catalog"?"Каталог":view==="learning"?"Моё обучение":"Мои курсы"}</h1>
   <p>{view==="catalog"?"Выберите курс для обучения.":view==="learning"?"Продолжайте обучение и управляйте сертификатами.":"Выберите курс или создайте новый. Дальше — простой редактор по шагам."}</p>
   <div className="how-grid">
    {view==="catalog"&&<section className="panel"><h2>Доступные курсы</h2>
     {catalogError?<p role="alert">Каталог временно недоступен: {catalogError}. Ваши курсы и обучения можно открыть отдельно.</p>:null}
     {!catalogError&&catalog.length===0?<p>Опубликованных курсов пока нет.</p>:catalog.map(c=>
      <article key={c.id}><h3>{c.title}</h3><p>{c.description}</p>
       <p>Версия {c.version_no}</p>
       {c.access_mode==="FREE"&&<p><strong>Бесплатно</strong> — автор открыл курс для свободного обучения.</p>}
       {c.access_mode==="PAID"&&<p><strong>{((c.price_minor||0)/100).toLocaleString("ru-RU")} ₽</strong> — платный курс специалиста.</p>}
       {(c.access_mode==="FREE"||verifiedCourseIDs.includes(c.id))?
        <button type="button" disabled={busy} onClick={()=>void act(
         ()=>api<Enrollment>(`/v1/academy/courses/${c.id}/enroll`,{}),
         async e=>{
          try {
           await loadLearning(e);
           setNotice("Право на обучение подтверждено.");
          } catch {
           // Enrollment POST succeeded: a separate lesson read outage must
           // not be described as a failed enrollment or trigger another POST.
           setView("learning");
           setActiveEnrollment(null);
           setNotice("Зачисление подтверждено сервером, но уроки временно не открылись. Найдите курс в «Моём обучении» и нажмите «Продолжить» позже.");
          }
         })}>
         {c.access_mode==="FREE"?"Начать бесплатно":"Начать обучение"}
        </button>:
        c.access_mode==="PAID"?
         <div>
          <button type="button" disabled={busy} onClick={async()=>{
           setCheckoutProblem(prev=>({...prev,[c.id]:""}));
           try{
            await api(`/v1/academy/courses/${c.id}/checkout`,{});
            setCheckoutProblem(prev=>({...prev,[c.id]:"Проверьте статус заказа: платёжный провайдер не подтвердил покупку."}));
           }catch(error){setCheckoutProblem(prev=>({...prev,[c.id]:error instanceof Error?error.message:"Оплата пока недоступна."}))}
          }}>Купить курс</button>
          {checkoutProblem[c.id]&&<p role="alert">{checkoutProblem[c.id]}</p>}
          <p>Оплату выполняет внешний банк. APGIC не получает деньги. Т-Банк и мультирасчёты пока не подключены.</p>
         </div>:
         <p>Автор ещё не открыл запись на этот курс.</p>}
      </article>)}
     <p>Доступ предоставляется только по подтверждённому entitlement. Если покупки нет, платформа не создаёт фиктивную оплату.</p>
    </section>}
    {view==="learning"&&<section className="panel"><h2>Мои обучения</h2>
     <button disabled={busy} onClick={()=>void act(()=>downloadAcademyHistory(),async()=>{setNotice("История обучения выгружена.")})}>Скачать историю обучения (JSON)</button>
     <section aria-label="Уведомления об обучении">
      <h3>Уведомления</h3>
      <button disabled={busy} onClick={()=>void act(
       ()=>api<AcademyNotifications>("/v1/academy/me/notifications"),setNotifications)}>Настроить уведомления</button>
      {notifications&&<form onSubmit={event=>{event.preventDefault();void act(
       ()=>api<AcademyNotifications>("/v1/academy/me/notifications",notifications),
       setNotifications)}}>
       {(["enabled","learning_updates","assessment_results","certificate_updates"] as const).map(field=>
        <label key={field}><input type="checkbox" checked={notifications[field]}
         onChange={event=>setNotifications(prev=>prev?{...prev,[field]:event.target.checked}:prev)}/>
         {{enabled:"Разрешить уведомления",learning_updates:"Новые уроки",assessment_results:"Оценки",certificate_updates:"Сертификаты"}[field]}</label>)}
       <button disabled={busy}>Сохранить согласия</button>
      </form>}
     </section>
     <section aria-label="Мои сертификаты">
      <h3>Мои сертификаты</h3>
      {certificatesUnavailable?<p role="alert">История сертификатов временно недоступна.</p>:
       certificates.length===0?<p>Выданных сертификатов пока нет.</p>:
       certificates.map(item=><p key={item.id}>
        {item.course_title} · версия {item.course_version} · сертификат {item.state==="REVOKED"?"отозван":"действует"} · доступ к урокам {item.learning_access_state==="ACTIVE"?"активен":"отозван"}.
        {" "}<a href={item.verification_url} rel="noreferrer">Проверить сертификат</a>
       </p>)}
     </section>
     {learningError?<p role="alert">История обучения временно недоступна: {learningError}. Проверьте вход и повторите загрузку.</p>:enrollments.length===0?<p>Пока нет зачислений.</p>:enrollments.map(e=>
      <div key={e.id}><p>{e.course_title||"Курс"} · версия {e.version_no??"—"} · {e.state} · доступ {e.access_state??"неизвестен"}</p>
       <button disabled={busy||e.access_state!=="ACTIVE"} onClick={()=>void act(
        ()=>loadLearning(e),()=>{})}>Продолжить</button>
      </div>)}
    </section>}
    {view==="courses"&&!course&&showCreate&&<form id="academy-author" className="panel specialist-form" onSubmit={createCourse}>
     <h2>Создать курс</h2>
     <p>Сначала создайте содержание курса. Настройка организации, направления и продукта для продаж доступна отдельно — в едином кабинете организации.</p>
     <a className="academy-organization-link" href="/organization">Настроить организацию и продажи ↗</a>
     <label htmlFor="academy-title">Название курса</label>
     <input id="academy-title" value={title} onChange={e=>setTitle(e.target.value)} required minLength={1} maxLength={200} placeholder="Например: Гипноз с нуля"/>
     <label htmlFor="academy-description">Описание (необязательно)</label>
     <textarea id="academy-description" value={description} onChange={e=>setDescription(e.target.value)}/>
     {authorSessionMissing&&<div role="alert" className="academy-session-guide">
       <p>На этом устройстве нет сессии APGIC. Чтобы сохранить курс в существующем аккаунте, сначала <a href="/account">подключите это устройство к своему аккаунту</a>.</p>
       <p>Если у вас ещё нет аккаунта с курсами, можно явно начать новый. Он не объединится с прежними курсами другого аккаунта.</p>
       <button type="button" disabled={busy} onClick={()=>void startAuthorSession()}>Начать новый аккаунт автора</button>
      </div>}
      <button className="academy-blue-button" disabled={busy||pendingDraft!==null||confirmedDraft!==null||authorSessionMissing||title.trim().length<1}>Создать курс</button>
     {pendingDraft&&<div role="alert" className="academy-draft-recovery">
       <strong>Не удалось подтвердить сохранение курса.</strong>
       <p>Повторное создание временно заблокировано, чтобы не дублировать курс. Кнопка ниже проверит сохранение и безопасно повторит тот же запрос при необходимости.</p>
       <button className="academy-draft-check" type="button" disabled={busy} onClick={()=>void verifyDraft()}>Проверить сохранение черновика</button>
      </div>}
     {confirmedDraft&&!pendingDraft&&<div role="status" className="academy-draft-recovery">
       <strong>«{confirmedDraft.title}» создан.</strong>
       <p>Повторно создавать курс не нужно — достаточно открыть существующий черновик.</p>
       <button className="academy-draft-check" type="button" disabled={busy} onClick={()=>void act(
        ()=>Promise.resolve(confirmedDraft),async selected=>{await loadOutline(selected);setConfirmedDraft(null)})}>Открыть созданный курс</button>
      </div>}
    </form>}
   </div>
  </section>}
  {view==="courses"&&!course&&<section className="how-section academy-course-list">
   <div className="academy-section-top"><h2>Ваши курсы</h2><button type="button" onClick={()=>setShowCreate(x=>!x)}>{showCreate?"Скрыть создание":"+ Новый курс"}</button></div>
   <p><a href="/account">Войти в тот же аккаунт с телефона или другого компьютера</a></p>
   {ownedCoursesError?<p role="alert">Не удалось загрузить ваши курсы: {ownedCoursesError}. Проверьте авторизацию на этом устройстве. Курсы другого аккаунта не отображаются автоматически.</p>:null}
   {ownedCoursesLoaded&&!ownedCoursesError&&mine.length===0?<p role="status">В текущей сессии нет авторских курсов. Если курсы есть на другом устройстве, возможно, там используется другая учётная запись. Не создавайте их повторно до проверки доступа к исходному аккаунту.</p>:null}
   <div className="how-grid">
    {mine.map(c=><article className="panel" key={c.id}>
     <h3>{c.title}</h3><p>Версия {c.version_no} · {c.state}</p>
     <button disabled={busy} onClick={()=>void act(
      ()=>Promise.resolve(c),
      async()=>{setEditorStep("structure");await loadOutline(c)})}>Открыть редактор</button>
     <button type="button" disabled={busy} onClick={()=>void openAuthorPreview(c)}>{c.state==="PUBLISHED"?"Посмотреть курс":"Предпросмотр"}</button>
     <button disabled={busy} onClick={()=>void act(
      ()=>api<Course>(`/v1/academy/courses/${c.id}/versions`,{}),
      async d=>loadOutline({...c,version_id:d.version_id,version_no:d.version_no}))}>Новая версия</button>
    </article>)}
   </div>
  </section>}
  {view==="courses"&&course&&<section className="how-section academy-course-editor">
   <button type="button" className="academy-return" onClick={()=>setCourse(null)}>← Все курсы</button>
   <p className="academy-overline">Версия {course.version_no} · {course.state==="DRAFT"?"Черновик":"Опубликован"}</p>
   <h1>{course.title}</h1>
   <div className="academy-action-buttons"><button type="button" className="academy-blue-button" disabled={busy} onClick={()=>void openAuthorPreview(course)}>{course.state==="PUBLISHED"?"Посмотреть курс":"Предпросмотр"}</button>{editorStep==="preview"&&<button type="button" onClick={()=>setEditorStep("structure")}>Вернуться к редактированию</button>}</div>
   <nav className="academy-steps" aria-label="Редактирование курса">
    <button type="button" aria-current={editorStep==="structure"?"step":undefined} onClick={()=>setEditorStep("structure")}>1. Уроки</button>
    <button type="button" aria-current={editorStep==="media"?"step":undefined} disabled={!modules.length||course.state!=="DRAFT"} onClick={()=>setEditorStep("media")}>2. Видео</button>
    <button type="button" aria-current={editorStep==="publish"?"step":undefined} disabled={!lessons.length} onClick={()=>setEditorStep("publish")}>3. Публикация</button>
   </nav>
   {editorStep==="preview"&&<section className="academy-structure" aria-label="Предпросмотр курса">
    <section className="panel academy-outline-tree">
     <h2>Содержание курса</h2>
     <p>Режим автора · версия {course.version_no}. Покупки, зачисления и прогресс не создаются.</p>
     {modules.length===0&&<p>В курсе пока нет модулей.</p>}
     {[...modules].sort((a,b)=>a.ordinal-b.ordinal).map(m=><div key={m.id}>
      <h3>{m.title}</h3>
      {lessons.filter(l=>l.module_id===m.id).sort((a,b)=>a.ordinal-b.ordinal).map(l=>
       <button key={l.id} type="button" className="academy-module-heading" aria-current={previewLessonID===l.id?"true":undefined} onClick={()=>setPreviewLessonID(l.id)}>{l.ordinal}. {l.title}</button>)}
     </div>)}
    </section>
    <section className="panel academy-outline-action">
     {course.description&&<p>{course.description}</p>}
     {(()=>{const ordered=[...modules].sort((a,b)=>a.ordinal-b.ordinal).flatMap(m=>lessons.filter(l=>l.module_id===m.id).sort((a,b)=>a.ordinal-b.ordinal));
      const index=Math.max(0,ordered.findIndex(l=>l.id===previewLessonID));
      const lesson=ordered[index];
      if(!lesson)return <p>Добавьте уроки, чтобы проверить курс.</p>;
      return <article key={lesson.id}>
       <p>Урок {index+1} из {ordered.length}</p><h2>{lesson.title}</h2>
       {lesson.kind==="READING"&&<p style={{whiteSpace:"pre-wrap"}}>{lesson.body}</p>}
       {lesson.kind==="VIDEO"&&<AcademyLessonVideos key={course.version_id+":"+lesson.id} authorVersionId={course.version_id} lessonId={lesson.id} available={true}/>}
       {lesson.kind==="ASSESSMENT"&&<div><p>Задание · проходной балл: {lesson.passing_score}</p>{lesson.body&&<p style={{whiteSpace:"pre-wrap"}}>{lesson.body}</p>}<label>Ответ ученика (предпросмотр)<textarea disabled placeholder="В режиме автора ответы не отправляются"/></label></div>}
       <div className="academy-action-buttons"><button type="button" disabled={index===0} onClick={()=>setPreviewLessonID(ordered[index-1].id)}>← Предыдущий урок</button><button type="button" disabled={index===ordered.length-1} onClick={()=>setPreviewLessonID(ordered[index+1].id)}>Следующий урок →</button></div>
      </article>
     })()}
    </section>
   </section>}
   {editorStep==="structure"&&<div className="academy-structure">
    <section className="panel academy-outline-tree" aria-label="Дерево курса">
     <div className="academy-tree-heading">
      <div><p className="academy-overline">Содержание</p><h2>Модули и уроки</h2></div>
      <button type="button" disabled={busy||course.state!=="DRAFT"}
       onClick={()=>setStructureAction("module")}>＋ Модуль</button>
     </div>
     {modules.length===0?<div className="academy-empty-outline">
      <strong>Пока нет модулей</strong>
      <p>Начните с раздела курса. Например: «Основы», затем добавьте в него уроки.</p>
      <button type="button" disabled={busy||course.state!=="DRAFT"} onClick={()=>setStructureAction("module")}>Создать первый модуль</button>
     </div>:<div className="academy-module-list">
      {[...modules].sort((a,b)=>a.ordinal-b.ordinal).map(m=><article key={m.id} className={moduleID===m.id?"academy-module-selected":""}>
       <button type="button" className="academy-module-heading" aria-current={moduleID===m.id?"true":undefined}
        onClick={()=>{setModuleID(m.id);setStructureAction("overview")}}>
        <span className="academy-module-number">{m.ordinal}</span><strong>{m.title}</strong>
        <small>{lessons.filter(l=>l.module_id===m.id).length} ур.</small>
       </button>
       {lessons.filter(l=>l.module_id===m.id).sort((a,b)=>a.ordinal-b.ordinal).map((l,i)=>
        <div className="academy-lesson-line" key={l.id}>
         <span className="academy-lesson-icon" aria-hidden="true">{l.kind==="VIDEO"?"▶":l.kind==="READING"?"▤":"✓"}</span>
         <span className="academy-lesson-name">{i+1}. {l.title}<small>{l.kind==="VIDEO"?"Видеоурок":l.kind==="READING"?"Текст":"Задание"}</small></span>
         {l.kind==="VIDEO"&&<button type="button" className="academy-lesson-video"
          disabled={busy} onClick={()=>{if(course.state==="DRAFT"){setFocusedVideoLessonID(l.id);setEditorStep("media")}else void openAuthorPreview(course,l.id)}}>
          Видео →</button>}
        </div>)}
       {course.state==="DRAFT"&&<button type="button" className="academy-add-lesson"
        onClick={()=>{setModuleID(m.id);setStructureAction("lesson")}}>＋ Добавить урок</button>}
      </article>)}
     </div>}
    </section>
    <section className="panel academy-outline-action" aria-label="Действия с курсом">
     {structureAction==="overview"&&<>
      <p className="academy-overline">Следующее действие</p>
      <h2>{modules.length===0?"Создайте первый модуль":lessons.length===0?"Добавьте первый урок":"Редактирование курса"}</h2>
      <p>{modules.length===0?"Модуль — это раздел курса, объединяющий несколько уроков.":
       lessons.length===0?"Выберите модуль слева и создайте в нём первый урок.":
       "Выберите модуль или урок слева. Видео прикрепляется к конкретному видеоуроку."}</p>
      <div className="academy-action-summary">
       <span><strong>{modules.length}</strong> модулей</span><span><strong>{lessons.length}</strong> уроков</span>
       <span><strong>{lessons.filter(l=>l.kind==="VIDEO").length}</strong> видеоуроков</span>
      </div>
      <button type="button" className="academy-blue-button" disabled={busy||course.state!=="DRAFT"}
       onClick={()=>setStructureAction(modules.length?"lesson":"module")}>
       {modules.length?"＋ Добавить урок":"＋ Создать модуль"}</button>
      {modules.length>0&&<button type="button" className="academy-secondary-action" onClick={()=>setStructureAction("module")} disabled={busy||course.state!=="DRAFT"}>Добавить ещё модуль</button>}
     </>}
     {structureAction==="module"&&<form className="academy-compact-form" onSubmit={addModule}>
      <p className="academy-overline">Новый раздел</p><h2>Добавить модуль</h2>
      <p>Например: «Теория», «Практика», «Итоговые задания».</p>
      <label htmlFor="academy-module">Название модуля</label>
      <input id="academy-module" value={moduleTitle} onChange={e=>setModuleTitle(e.target.value)} required maxLength={200} autoFocus placeholder="Например: Основы"/>
      <div className="academy-action-buttons"><button type="submit" className="academy-blue-button" disabled={busy||course.state!=="DRAFT"||!moduleTitle.trim()}>Сохранить модуль</button>
       <button type="button" onClick={()=>setStructureAction("overview")}>Отмена</button></div>
     </form>}
     {structureAction==="lesson"&&<form className="academy-compact-form" onSubmit={addLesson}>
      <p className="academy-overline">Новый материал</p><h2>Добавить урок</h2>
      <label htmlFor="academy-module-select">В какой модуль?</label>
      <select id="academy-module-select" value={moduleID} onChange={e=>setModuleID(e.target.value)} required>
       {modules.map(m=><option key={m.id} value={m.id}>{m.ordinal}. {m.title}</option>)}
      </select>
      <label htmlFor="academy-lesson-title">Название урока</label>
      <input id="academy-lesson-title" value={lessonTitle} onChange={e=>setLessonTitle(e.target.value)} required maxLength={200} autoFocus placeholder="Например: Введение"/>
      <label htmlFor="academy-lesson-kind">Формат урока</label>
      <select id="academy-lesson-kind" value={lessonKind} onChange={e=>setLessonKind(e.target.value as Lesson["kind"])}>
       <option value="VIDEO">Видео</option><option value="READING">Текст</option><option value="ASSESSMENT">Проверочное задание</option>
      </select>
      {lessonKind!=="VIDEO"&&<><label htmlFor="academy-lesson-body">Содержание урока</label>
       <textarea id="academy-lesson-body" value={lessonBody} onChange={e=>setLessonBody(e.target.value)}/></>}
      {lessonKind==="ASSESSMENT"&&<><label htmlFor="academy-pass">Проходной балл</label>
       <input id="academy-pass" type="number" min={1} max={100} value={passingScore} onChange={e=>setPassingScore(Number(e.target.value))}/></>}
      {lessonKind==="VIDEO"&&<p className="academy-form-tip">После создания сразу откроется загрузка видео именно в этот урок.</p>}
      <div className="academy-action-buttons"><button type="submit" className="academy-blue-button" disabled={busy||!moduleID||course.state!=="DRAFT"||!lessonTitle.trim()}>Сохранить урок</button>
       <button type="button" onClick={()=>setStructureAction("overview")}>Отмена</button></div>
     </form>}
    </section>
    <div className="academy-extra-settings">
     <h2>Дополнительные настройки</h2>
     <p>Здесь можно настроить даты открытия уроков и правила пересдач. Права на материалы подтверждаются на шаге «Публикация».</p>
     {course.state==="DRAFT"&&<details className="academy-advanced-details" aria-label="Расписание и лимиты уроков"><summary>Расписание и ограничения уроков</summary><section>
      <h4>Расписание уроков и пересдачи</h4>
      {lessons.map(lesson=><div key={lesson.id}>
       <h5>{lesson.title}</h5>
       <form onSubmit={event=>{event.preventDefault();const data=new FormData(event.currentTarget);
        const release=String(data.get("release")||"");
        const due=String(data.get("deadline")||"");
        void act(()=>api(`/v1/academy/lessons/${lesson.id}/schedule`,{
         release_at:release?new Date(release).toISOString():null,
         deadline_at:due?new Date(due).toISOString():null
        }),()=>setNotice("Расписание урока сохранено."))}}>
        <label>Открыть урок <input name="release" type="datetime-local" required/></label>
        <label>Закрыть урок (необязательно) <input name="deadline" type="datetime-local"/></label>
        <button disabled={busy}>Сохранить расписание</button>
       </form>
       {lesson.kind==="ASSESSMENT"&&<form onSubmit={event=>{event.preventDefault();
        const data=new FormData(event.currentTarget);const due=String(data.get("deadline")||"");
        void act(()=>api(`/v1/academy/lessons/${lesson.id}/assessment-limits`,{
         max_attempts:Number(data.get("max_attempts")),
         submission_deadline:due?new Date(due).toISOString():null
        }),()=>setNotice("Ограничения экзамена сохранены."))}}>
        <label>Максимум попыток <input name="max_attempts" type="number" min={1} max={100} defaultValue={1} required/></label>
        <label>Последний срок сдачи <input name="deadline" type="datetime-local"/></label>
        <button disabled={busy}>Сохранить правила экзамена</button>
       </form>}
      </div>)}
     </section></details>}
    </div>
   </div>}
   {editorStep==="structure"&&<div className="academy-next"><button type="button" className="academy-blue-button" disabled={!modules.length||course.state!=="DRAFT"} onClick={()=>setEditorStep("media")}>Далее: видео →</button></div>}
   {editorStep==="media"&&course.state==="DRAFT"&&<CloudMediaEditor key={`${course.id}:${course.version_id}`} courseId={course.id} versionId={course.version_id} modules={mediaModuleOptions} focusedLessonId={focusedVideoLessonID} lessons={mediaLessonOptions}/>}
   {editorStep==="media"&&<div className="academy-next"><button type="button" onClick={()=>setEditorStep("structure")}>← К урокам</button><button type="button" className="academy-blue-button" onClick={()=>setEditorStep("publish")}>Далее: публикация →</button></div>}
   {editorStep==="publish"&&<section className="panel academy-publish-overview"><h2>1. Проверьте содержание курса</h2><p>Модулей: {modules.length} · уроков: {lessons.length} · видеоуроков: {lessons.filter(l=>l.kind==="VIDEO").length}</p><p>Перед публикацией проверьте содержание и прикреплённые видео. После публикации эта версия станет неизменяемой.</p><div className="academy-publish-review-actions"><button type="button" onClick={()=>setEditorStep("structure")}>Проверить уроки</button><button type="button" onClick={()=>{if(course.state==="DRAFT")setEditorStep("media");else void openAuthorPreview(course,lessons.find(l=>l.kind==="VIDEO")?.id)}}>Проверить видео</button><button type="button" onClick={()=>void openAuthorPreview(course)}>Открыть предпросмотр</button></div></section>}
   {editorStep==="publish"&&course.state==="DRAFT"&&<section className="panel academy-publish-rights" aria-label="Декларация авторских прав">
      <h2>2. Подтвердите авторские права</h2>
      <p>Один раз укажите, откуда материалы курса, и подтвердите своё право их публиковать.</p>
      <label htmlFor="academy-original-rights">Источник материалов</label>
      <input id="academy-original-rights" ref={rightsInputRef} value={rightsSource}
       aria-describedby="academy-original-rights-hint"
       placeholder="Например: мои авторские лекции и видеозаписи"
       onChange={e=>{setRightsSource(e.target.value);setRightsRecorded(false);setRightsFeedback("");setPublishError("")}}/>
      <p id="academy-original-rights-hint">Назовите источник материалов как удобно, например «№1» или «мои лекции». Галочка сама по себе не сохраняет подтверждение.</p>
      <label className="academy-rights-checkbox"><input type="checkbox" checked={rightsAgreed}
       onChange={e=>{setRightsAgreed(e.target.checked);setRightsFeedback("");setPublishError("")}}/>
       Подтверждаю авторство и право публикации</label>
      <button type="button" className="academy-blue-button" disabled={busy}
       onClick={async()=>{
        setRightsFeedback("");setPublishError("");
        if(!rightsSource.trim()){
         setRightsFeedback("Укажите источник материалов. Можно назвать его даже «№1». ");
         rightsInputRef.current?.focus();
         return;
        }
        if(!rightsAgreed){
         setRightsFeedback("Поставьте отметку о праве публикации.");
         return;
        }
        if(lessons.length===0){
         setRightsFeedback("Сначала добавьте хотя бы один урок.");
         return;
        }
        setBusy(true);
        try{
         await api<{id:string}>(`/v1/academy/courses/${course.id}/versions/${course.version_id}/rights/original`,
          {provenance_ref:rightsSource.trim(),acknowledged:true});
         const checked=await api<{confirmed:boolean;provenance_ref:string}>(
          `/v1/academy/courses/${course.id}/versions/${course.version_id}/rights/original`);
         if(!checked.confirmed){
          throw new Error("Подтверждение не найдено в сохранённых данных этой версии. Проверьте уроки и повторите.");
         }
         // Repeated submissions return the original immutable signed record.
         // Readback, not the editable text, determines exact-version validity.
         setRightsSource(checked.provenance_ref);
         setRightsRecorded(true);
         setRightsFeedback(checked.provenance_ref===rightsSource.trim()
          ?"Права подтверждены. Теперь можно опубликовать курс."
          :"Права уже подтверждены для этой версии. Используется ранее сохранённый источник: «"+checked.provenance_ref+"».");
        }catch(error){
         setRightsRecorded(false);
         setRightsFeedback(error instanceof Error?error.message:"Не удалось сохранить подтверждение.");
        }finally{setBusy(false)}
       }}>Подтвердить права</button>
      {rightsFeedback&&<p className={rightsRecorded?"academy-inline-result":"academy-inline-error"} role="status">{rightsFeedback}</p>}
     </section>}
   {editorStep==="publish"&&course.state==="DRAFT"&&<div className="academy-publish-submit">
    <button type="button" className="academy-blue-button" disabled={busy||lessons.length===0}
     onClick={async()=>{
      if(busy)return;
      if(!rightsSource.trim()){
       setPublishError("Укажите источник материалов. Подойдёт любое непустое название, даже «№1».");
       rightsInputRef.current?.scrollIntoView({behavior:"smooth",block:"center"});
       rightsInputRef.current?.focus({preventScroll:true});
       return;
      }
      if(!rightsAgreed){
       setPublishError("Поставьте отметку «Подтверждаю авторство и право публикации» выше.");
       return;
      }
      setPublishError("");setNotice("");setBusy(true);
      try{
       const rightsPath=`/v1/academy/courses/${course.id}/versions/${course.version_id}/rights/original`;
       // Publish must be authorized by the exact-version server record,
       // not by local checkbox state. Save consent when not yet recorded.
       if(!rightsRecorded){
        await api<{id:string}>(rightsPath,{provenance_ref:rightsSource.trim(),acknowledged:true});
       }
       const stored=await api<{confirmed:boolean;provenance_ref:string}>(rightsPath);
       if(!stored.confirmed){
        setRightsRecorded(false);
        throw new Error("Права не сохранены для текущего содержания. Подтвердите их снова после проверки уроков.");
       }
       setRightsSource(stored.provenance_ref);
       setRightsRecorded(true);
       const published=await api<Course>(`/v1/academy/courses/${course.id}/versions/${course.version_id}/publish`,{});
       if(published.state!=="PUBLISHED"||published.id!==course.id||published.version_id!==course.version_id){
        throw new Error("Проверка результата не удалась. Откройте «Мои курсы» и проверьте статус.");
       }
       setMine(previous=>previous.map(item=>item.id===published.id?published:item));
       setCourse(null);setNotice("Курс опубликован. Настройки доступа и цены можно изменить отдельно.");
       window.scrollTo({top:0,behavior:"smooth"});
       void refresh().catch(()=>setNotice("Курс опубликован. Не удалось обновить список — откройте «Мои курсы»."));
      }catch(error){
       const detail=error instanceof Error?error.message:"Неизвестная ошибка";
       setPublishError(detail==="Недопустимое изменение состояния."||detail==="ACADEMY_STATE_CONFLICT"?
        "Публикация не выполнена. Проверьте подтверждение прав и содержание курса.":"Публикация не выполнена: "+detail);
      }finally{setBusy(false)}
     }}>Опубликовать курс</button>
    {publishError&&<p className="academy-publish-error" role="alert">{publishError}</p>}
   </div>}
  </section>}
  {view==="courses"&&course&&<section className="academy-course-offer" aria-label="Доступ и стоимость курса">
   <div className="panel academy-offer-panel">
    <h2>Доступ к курсу</h2>
    <p className="academy-offer-current">Условия: <strong>{course.access_mode==="FREE"?"Бесплатно":course.access_mode==="PAID"?"Платный · "+((course.price_minor||0)/100).toLocaleString("ru-RU")+" ₽":"Доступ не открыт"}</strong>{course.state!=="PUBLISHED"&&<span> · курс пока черновик</span>}</p>
    <form onSubmit={async e=>{
     e.preventDefault();
     const rub=Number(offerPriceRub);
     if(offerMode==="PAID"&&(!Number.isSafeInteger(rub)||rub<1||rub>100000000)){
      setOfferFeedback({kind:"error",message:"Укажите цену целым числом от 1 до 100 000 000 ₽."});return;
     }
     setOfferFeedback(null);setBusy(true);
     try{
      const saved=await api<CourseOffer>(`/v1/academy/courses/${course.id}/offer`,{
       access_mode:offerMode,price_minor:offerMode==="FREE"?0:rub*100
      });
      const readback=await api<{courses:Course[]}>("/v1/academy/my-courses");
      const confirmed=readback.courses.find(item=>item.id===course.id);
      if(!confirmed||confirmed.access_mode!==saved.access_mode||confirmed.price_minor!==saved.price_minor){
       throw new Error("Сохранение не подтверждено сервером. Перезагрузите список курсов.");
      }
      setCourse(old=>old?{...old,access_mode:confirmed.access_mode,price_minor:confirmed.price_minor,currency:confirmed.currency}:old);
      setMine(readback.courses);
      // The author readback is the canonical saved state. Keep the catalog
      // projection in sync immediately, including newly opened courses.
      setCatalog(items=>{
       const other=items.filter(item=>item.id!==confirmed.id);
       return confirmed.state==="PUBLISHED"&&
        (confirmed.access_mode==="FREE"||confirmed.access_mode==="PAID")?
        [...other,confirmed]:other;
      });
      setOfferFeedback({kind:"success",message:saved.access_mode==="FREE"?
       (course.state==="PUBLISHED"?"Сохранено: курс доступен бесплатно.":"Сохранено: бесплатный доступ после публикации курса."):"Сохранено: "+(saved.price_minor/100).toLocaleString("ru-RU")+" ₽. Приём платежей пока не подключён."});
     }catch(error){
      setOfferFeedback({kind:"error",message:error instanceof Error?error.message:"Не удалось сохранить условия доступа."});
     }finally{setBusy(false)}
    }}>
     <fieldset className="academy-offer-choice"><legend>Как открыть курс?</legend>
      <label><input type="radio" name="academy-access-mode" checked={offerMode==="FREE"}
       onChange={()=>{setOfferMode("FREE");setOfferFeedback(null)}}/> <span>Открытый <small>Бесплатно для учеников</small></span></label>
      <label><input type="radio" name="academy-access-mode" checked={offerMode==="PAID"}
       onChange={()=>{setOfferMode("PAID");setOfferFeedback(null)}}/> <span>Платный <small>Доступ после оплаты</small></span></label>
     </fieldset>
     {offerMode==="PAID"&&<label className="academy-offer-price">Стоимость курса, ₽
      <input type="number" min={1} max={100000000} step={1} required value={offerPriceRub}
       onChange={e=>{setOfferPriceRub(e.target.value);setOfferFeedback(null)}} aria-label="Цена курса в рублях"/>
     </label>}
     <button type="submit" className="academy-blue-button" disabled={busy}>
      {busy?"Сохраняем…":"Сохранить"}
     </button>
     {offerFeedback&&<p className={offerFeedback.kind==="success"?"academy-inline-result":"academy-inline-error"} role="status">{offerFeedback.message}</p>}
    </form>
    {offerMode==="PAID"&&<p className="academy-offer-note">Оплата заработает после подключения Т-Банка. Деньги не проходят через APGIC.</p>}
   </div>
  </section>}
  {view==="courses"&&course&&course.state==="PUBLISHED"&&<section className="how-section">
   <h2>Учебные потоки преподавателя</h2>
   <form className="panel specialist-form" onSubmit={e=>{e.preventDefault();void act(
    ()=>api<Cohort>(`/v1/academy/versions/${course.version_id}/cohorts`,{
     label:cohortTitle,capacity:cohortCapacity,starts_at:new Date(cohortStarts).toISOString(),
     enrollment_closes_at:cohortCloses?new Date(cohortCloses).toISOString():null
    }),async()=>{setCohortTitle("");setNotice("Учебный поток создан.")})}}>
    <label htmlFor="r8-cohort-title">Название потока</label>
    <input id="r8-cohort-title" value={cohortTitle} onChange={e=>setCohortTitle(e.target.value)} required/>
    <label htmlFor="r8-cohort-capacity">Количество мест</label>
    <input id="r8-cohort-capacity" type="number" min={1} max={100000} value={cohortCapacity}
     onChange={e=>setCohortCapacity(Number(e.target.value))} required/>
    <label htmlFor="r8-cohort-start">Начало занятий</label>
    <input id="r8-cohort-start" type="datetime-local" value={cohortStarts}
     onChange={e=>setCohortStarts(e.target.value)} required/>
    <label htmlFor="r8-cohort-close">Конец записи (необязательно)</label>
    <input id="r8-cohort-close" type="datetime-local" value={cohortCloses}
     onChange={e=>setCohortCloses(e.target.value)}/>
    <button disabled={busy||!cohortStarts}>Создать поток</button>
   </form>
   {authorCohorts.filter(h=>h.version_id===course.version_id).map(h=><p key={h.id}>{h.label}: {h.members} из {h.capacity} мест</p>)}
  </section>}
  {view==="learning"&&activeEnrollment&&<section className="how-section">
   <h2>Прохождение курса</h2>
   {attemptHistoryError&&<p role="alert">История выполненных заданий временно недоступна. Уроки можно изучать; повторите открытие курса, чтобы загрузить историю.</p>}
   <h3>Доступные учебные потоки</h3>
   {cohortError?<p role="alert">Список потоков временно недоступен; обучение продолжается.</p>:cohorts.length===0?<p>Потоки для этой версии курса ещё не объявлены.</p>:cohorts.map(h=><article className="panel" key={h.id}>
    <strong>{h.label}</strong><p>Мест: {h.members} из {h.capacity}; начало: {h.starts_at?new Date(h.starts_at).toLocaleString("ru-RU"):"не задано"}</p>
    <button disabled={busy||h.is_member||h.members>=h.capacity} onClick={()=>void act(
     ()=>api(`/v1/academy/cohorts/${h.id}/join`,{enrollment_id:activeEnrollment.id}),
     async()=>{setNotice("Зачисление в поток подтверждено.");await loadLearning(activeEnrollment)})}>{h.is_member?"Вы уже записаны":"Записаться в поток"}</button>
   </article>)}

   <p>Материал доступен только владельцу активного права обучения.</p>
   <div className="how-grid">
    {lessons.map(l=><article className="panel" key={l.id}>
     <h3>{l.title}</h3><p>{l.kind} {l.completed?"· выполнено":""}</p>
     {l.available===false&&<p role="status">Урок ещё не открыт или срок его изучения истёк.</p>}
     {l.available!==false&&l.kind==="READING"&&<p style={{whiteSpace:"pre-wrap"}}>{l.body}</p>}
     {l.kind==="VIDEO"&&<AcademyLessonVideos enrollmentId={activeEnrollment.id} lessonId={l.id} available={l.available!==false}/>}
     {l.kind==="VIDEO"&&l.available!==false&&!l.completed&&<div>
      <p>После полного получения всех видео этого урока подтвердите изучение. Перемотка сама по себе не засчитывает недоставленные фрагменты.</p>
      <button disabled={busy} onClick={()=>void act(
       ()=>api(`/v1/academy/enrollments/${activeEnrollment.id}/lessons/${l.id}/complete`,{}),
       async()=>{await loadLearning(activeEnrollment);setNotice("Изучение видеоматериалов подтверждено.")})}>
       Подтвердить изучение видео
      </button>
     </div>}
     {l.available!==false&&l.kind==="ASSESSMENT"&&<section aria-label={`Результаты: ${l.title}`}>
      {learnerAttempts.filter(a=>a.lesson_id===l.id).map(a=>
       <p key={a.id}>Попытка {a.attempt_no}: {a.result==="PENDING"?"Ожидает проверки":a.result==="PASSED"?"Зачтено":"Не зачтено"}
        {a.score!==undefined?` · ${a.score}/100`:""}</p>)}
      {learnerAttempts.filter(a=>a.lesson_id===l.id&&(a.result==="FAILED"||a.result==="PASSED")).map(a=>
       <div key={`appeal-${a.id}`}>
        {learnerAppeals.some(x=>x.attempt_id===a.id)?
         <p>Апелляция: {learnerAppeals.find(x=>x.attempt_id===a.id)?.state}</p>:appealError?
         <p role="alert">История апелляций временно недоступна. Повторная подача заблокирована до проверки.</p>:
         <form onSubmit={event=>{event.preventDefault();const form=new FormData(event.currentTarget);
          void act(()=>api<LearnerAppeal>(`/v1/academy/attempts/${a.id}/appeals`,{
           reason:String(form.get("reason")||"")
          }),async()=>{await loadLearning(activeEnrollment);setNotice("Апелляция подана.")})}}>
          <label>Причина апелляции <textarea name="reason" minLength={10} maxLength={2000} required/></label>
          <button disabled={busy}>Оспорить оценку</button>
         </form>}
       </div>)}
      {learnerAttempts.some(a=>a.lesson_id===l.id&&(a.result==="PENDING"||a.result==="PASSED"))
       ?<p>Новая попытка недоступна до завершения проверки или после зачёта.</p>
       :<form onSubmit={e=>{e.preventDefault();void act(
        ()=>api<{id:string}>(`/v1/academy/enrollments/${activeEnrollment.id}/assessments/${l.id}/attempts`,
        {response:attemptResponse[l.id]||""}),async a=>{
         setAttemptResponse(previous=>({...previous,[l.id]:""}));
         await loadLearning(activeEnrollment);
         setNotice(`Работа отправлена на проверку. ID: ${a.id}`);
        })}}>
        <label htmlFor={`attempt-${l.id}`}>Ответ</label>
        <textarea id={`attempt-${l.id}`} value={attemptResponse[l.id]||""}
         onChange={e=>setAttemptResponse(previous=>({...previous,[l.id]:e.target.value}))} required/>
        <button disabled={busy}>Отправить на проверку</button>
       </form>}
     </section>}
     {l.available!==false&&l.kind==="READING"&&<button disabled={busy||l.completed} onClick={()=>void act(
      ()=>api(`/v1/academy/enrollments/${activeEnrollment.id}/lessons/${l.id}/complete`,{}),
      async()=>{await loadLearning(activeEnrollment)})}>Отметить выполнение</button>}
    </article>)}
   </div>
   <button disabled={busy} onClick={()=>void act(
    ()=>api<Certificate>(`/v1/academy/enrollments/${activeEnrollment.id}/complete`,{}),
    async cert=>{setCertificate(cert);setNotice("Курс завершён, сертификат выпущен.")})}>Завершить курс и получить сертификат</button>
   {certificate&&<p>Сертификат: <a href={certificate.verification_url}>{certificate.verification_url}</a>. Статус профессиональной верификации: {certificate.evidence_candidate_state}.</p>}
  </section>}
  {view==="reports"&&!authorReport&&<p role="status" className="panel">Отчёт пока недоступен. Обновите страницу или попробуйте позже.</p>}
  {view==="reports"&&authorReport&&<section className="how-section" aria-label="Результаты обучения и подтверждённые оплаты">
   <h2>Результаты моих курсов</h2>
   <div className="how-grid">{authorReport.courses.map(item=><article className="panel" key={item.course_id}>
    <h3>{item.title}</h3>
    <p>Зачислено: {item.enrollments} · Завершили: {item.completed} · Действующий доступ: {item.active_access}</p>
    <p>Задания: ожидают проверки {item.pending_assessments}, зачтено {item.passed_assessments}</p>
    {authorReport.gross_captures.filter(e=>e.course_id===item.course_id).map(e=>
     <p key={e.currency}>Подтверждённых оплат: {e.captured_count}; валовая сумма: {e.captured_gross_minor} {e.currency} (в минимальных денежных единицах).</p>)}
   </article>)}</div>
   <p>Валовая сумма подтверждённых списаний не является доходом автора. Возвраты, комиссии, доли правообладателей и выплаты рассчитывает только канонический финансовый контур. До его сверки чистый доход не показывается.</p>
  </section>}
  {view==="reviews"&&<section id="academy-reviews" className="how-section"><h2>Проверка заданий преподавателем</h2>
   {reviewError&&<p role="alert">Очередь проверки работ недоступна: {reviewError}. Проверка новых оценок заблокирована до подтверждения прав.</p>}
   <div className="how-grid">{pendingReviews.map(item=><article className="panel" key={item.attempt_id}>
    <h3>{item.course_title} — {item.lesson_title}</h3>
    <p>Попытка {item.attempt_no}, рубрика {item.rubric_version}, порог {item.passing_score}/100</p>
    <p style={{whiteSpace:"pre-wrap"}}>{item.response_text}</p>
    <button disabled={busy} onClick={()=>{setAttemptID(item.attempt_id);setEvaluationScore(item.passing_score)}}>Проверить эту работу</button>
   </article>)}</div>
   <form className="panel specialist-form" onSubmit={e=>{e.preventDefault();void act(
    ()=>api(`/v1/academy/attempts/${attemptID}/evaluate`,
    {score:evaluationScore,evidence_ref:evaluationEvidence}),
    async()=>{setNotice("Оценка сохранена с evidence, успешное задание включено в прогресс.");setAttemptID("")})}}>
    <label htmlFor="academy-attempt">ID попытки ученика</label>
    <input id="academy-attempt" value={attemptID} onChange={e=>setAttemptID(e.target.value)} required/>
    <label htmlFor="academy-score">Оценка от 0 до 100</label>
    <input id="academy-score" type="number" min={0} max={100} value={evaluationScore} onChange={e=>setEvaluationScore(Number(e.target.value))}/>
    <label htmlFor="academy-evidence">Ссылка на проверяемое доказательство решения (opaque reference)</label>
    <input id="academy-evidence" value={evaluationEvidence} onChange={e=>setEvaluationEvidence(e.target.value)} required/>
    <button disabled={busy||Boolean(reviewError)}>Зафиксировать оценку</button>
   </form>
  </section>}
  </div>
 </main>;
}
