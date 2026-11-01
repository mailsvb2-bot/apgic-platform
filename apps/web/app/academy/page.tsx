"use client";

import {useCallback,useEffect,useState,type FormEvent} from "react";
import CloudMediaEditor from "./cloud-media-editor";
import AcademyLessonVideos from "./academy-lesson-videos";

type AcademyNotifications = {enabled:boolean;learning_updates:boolean;assessment_results:boolean;certificate_updates:boolean};
type AuthorProduct = {id:string;name:string};
type AcademyOrg = {id:string;name:string;directions:{id:string;name:string;status:string}[]};
type AcademyOrgProduct = {id:string;name:string;status:string};
type Course = {id:string;product_id:string;title:string;description:string;state:string;version_id:string;version_no:number};
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
 if(!response.ok)throw new Error(data.message_safe||data.code||"Сервис временно недоступен");
 return data as T;
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
 const [catalog,setCatalog]=useState<Course[]>([]);
 const [mine,setMine]=useState<Course[]>([]);
 const [eligibleProducts,setEligibleProducts]=useState<AuthorProduct[]>([]);
 const [productLoadError,setProductLoadError]=useState("");
 const [orgs,setOrgs]=useState<AcademyOrg[]>([]);
 const [newOrgName,setNewOrgName]=useState("");
 const [newDirectionName,setNewDirectionName]=useState("Обучение");
 const [orgMutationUncertain,setOrgMutationUncertain]=useState(false);
 const [directionMutationUncertain,setDirectionMutationUncertain]=useState(false);
 const [productMutationUncertain,setProductMutationUncertain]=useState(false);
 const [orgID,setOrgID]=useState("");
 const [directionID,setDirectionID]=useState("");
 const [orgProducts,setOrgProducts]=useState<AcademyOrgProduct[]>([]);
 const [preparedProductID,setPreparedProductID]=useState("");
 const [commercialRef,setCommercialRef]=useState("");
 const [authorRefs,setAuthorRefs]=useState("");
 const [beneficiaryRef,setBeneficiaryRef]=useState("");
 const [productStepError,setProductStepError]=useState("");
 const [productBusy,setProductBusy]=useState(false);
 const [enrollments,setEnrollments]=useState<Enrollment[]>([]);
 const [verifiedCourseIDs,setVerifiedCourseIDs]=useState<string[]>([]);
 const [course,setCourse]=useState<Course|null>(null);
 const [modules,setModules]=useState<Module[]>([]);
 const [lessons,setLessons]=useState<Lesson[]>([]);
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
 const [notifications,setNotifications]=useState<AcademyNotifications|null>(null);
 const [productID,setProductID]=useState("");
 const [title,setTitle]=useState("");
 const [description,setDescription]=useState("");
 const [moduleTitle,setModuleTitle]=useState("");
 const [lessonTitle,setLessonTitle]=useState("");
 const [lessonBody,setLessonBody]=useState("");
 const [lessonKind,setLessonKind]=useState<Lesson["kind"]>("READING");
 const [moduleID,setModuleID]=useState("");
 const [passingScore,setPassingScore]=useState(80);
 const [attemptResponse,setAttemptResponse]=useState<Record<string,string>>({});
 const [learnerAttempts,setLearnerAttempts]=useState<LearnerAttempt[]>([]);
 const [learnerAppeals,setLearnerAppeals]=useState<LearnerAppeal[]>([]);
 const [attemptID,setAttemptID]=useState("");
 const [pendingReviews,setPendingReviews]=useState<ReviewAttempt[]>([]);
 const [authorReport,setAuthorReport]=useState<AuthorReport|null>(null);
 const [evaluationScore,setEvaluationScore]=useState(80);
 const [evaluationEvidence,setEvaluationEvidence]=useState("");
 const [rightsSource,setRightsSource]=useState("");
 const [rightsAgreed,setRightsAgreed]=useState(false);
 const [rightsRecorded,setRightsRecorded]=useState(false);

 const refresh=useCallback(async()=>{
  const publicCourses=await api<{courses:Course[]}>("/v1/academy/courses");
  setCatalog(publicCourses.courses);
  const [owned,learning,reviews,report,cohortList,eligible,access,credentials]=await Promise.allSettled([
   api<{courses:Course[]}>("/v1/academy/my-courses"),
   api<{enrollments:Enrollment[]}>("/v1/academy/my-enrollments"),
   api<{attempts:ReviewAttempt[]}>("/v1/academy/review/attempts"),
   api<AuthorReport>("/v1/academy/author/report"),
   api<{cohorts:Cohort[]}>("/v1/academy/author/cohorts"),
   api<{products:AuthorProduct[]}>("/v1/academy/author/eligible-products"),
   api<{course_ids:string[]}>("/v1/academy/me/course-access"),
   api<{certificates:LearnerCertificate[]}>("/v1/academy/me/certificates")
  ]);
  if(owned.status==="fulfilled")setMine(owned.value.courses);
  if(learning.status==="fulfilled")setEnrollments(learning.value.enrollments);
  if(reviews.status==="fulfilled")setPendingReviews(reviews.value.attempts);
  if(report.status==="fulfilled")setAuthorReport(report.value);
  if(cohortList.status==="fulfilled")setAuthorCohorts(cohortList.value.cohorts);
  if(eligible.status==="fulfilled"){
   setEligibleProducts(eligible.value.products);
   setProductLoadError("");
  }else{
   setEligibleProducts([]);
   setProductID("");
   setProductLoadError(eligible.reason instanceof Error?eligible.reason.message:"Не удалось загрузить Commerce-продукты");
  }
  // Fail closed on expired sessions or unavailable entitlement projection.
  setVerifiedCourseIDs(access.status==="fulfilled"?access.value.course_ids:[]);
  setCertificates(credentials.status==="fulfilled"?credentials.value.certificates:[]);
  setCertificatesUnavailable(credentials.status!=="fulfilled");
 },[]);
 useEffect(()=>{void refresh().catch(error=>setNotice(String(error)))},[refresh]);

 async function act<T>(work:()=>Promise<T>,after:(result:T)=>Promise<void>|void){
  setBusy(true);setNotice("");
  try{const result=await work();await after(result);await refresh()}
  catch(error){setNotice(error instanceof Error?error.message:"Операция не выполнена")}
  finally{setBusy(false)}
 }
 async function loadOutline(selected:Course){
  setCourse(selected);setActiveEnrollment(null);setCertificate(null);setLearnerAttempts([]);setCohorts([]);
  const d=await api<{state:string;modules:Module[];lessons:Lesson[]}>(
   `/v1/academy/courses/${selected.id}/versions/${selected.version_id}/outline`);
  setModules(d.modules);setLessons(d.lessons);setModuleID(d.modules[0]?.id||"");
 }
 async function loadLearning(item:Enrollment){
  const [d,history,cohortResult,appealList]=await Promise.all([
   api<{lessons:Lesson[]}>(`/v1/academy/enrollments/${item.id}/learn`),
   api<{attempts:LearnerAttempt[]}>(`/v1/academy/enrollments/${item.id}/attempts`),
   api<{cohorts:Cohort[]}>(`/v1/academy/enrollments/${item.id}/cohorts`)
    .then(value=>({ok:true,items:value.cohorts})).catch(()=>({ok:false,items:[] as Cohort[]})),
   api<{appeals:LearnerAppeal[]}>("/v1/academy/me/appeals")
    .then(value=>({ok:true,items:value.appeals})).catch(()=>({ok:false,items:[] as LearnerAppeal[]}))
  ]);
  setActiveEnrollment(item);setCourse(null);setCertificate(null);
  setLessons(d.lessons);setLearnerAttempts(history.attempts);setLearnerAppeals(appealList.items);setModules([]);
  setCohorts(cohortResult.items);setCohortError(!cohortResult.ok);setAppealError(!appealList.ok);
 }
 async function loadOrgProducts(id:string){
  if(!id){setOrgProducts([]);return;}
  const data=await api<{products:AcademyOrgProduct[]}>(`/v1/organizations/${id}/products`);
  setOrgProducts(data.products);
 }
 async function loadOrganizations(){
  setProductStepError("");
  try{
   const result=await api<{organizations:AcademyOrg[]}>("/v1/organizations");
   setOrgs(result.organizations);
   setOrgMutationUncertain(false);
   const first=result.organizations.find(o=>o.id===orgID)||result.organizations[0];
   setOrgID(first?.id||"");
   if(first){await loadOrgProducts(first.id)}else{setOrgProducts([])}
  }catch(e){setProductStepError(e instanceof Error?e.message:String(e))}
 }
 async function createAcademyOrganization(event:FormEvent){
  event.preventDefault();if(orgMutationUncertain||productBusy)return;
  setProductBusy(true);setProductStepError("");
  try{
   const created=await api<AcademyOrg>("/v1/organizations",{name:newOrgName});
   setOrgs(current=>[...current,created]);setOrgID(created.id);setNewOrgName("");
   setDirectionID("");setOrgProducts([]);
  }catch(e){
   setOrgMutationUncertain(true);
   setProductStepError("Создание организации не подтверждено. Обновите организации перед повтором: "+String(e));
  }finally{setProductBusy(false)}
 }
 async function createAcademyDirection(event:FormEvent){
  event.preventDefault();if(directionMutationUncertain||productBusy||!orgID)return;
  setProductBusy(true);setProductStepError("");
  try{
   const updated=await api<AcademyOrg>(`/v1/organizations/${orgID}/directions`,{
    name:newDirectionName,direction_type:"EDUCATION"
   });
   setOrgs(current=>current.map(o=>o.id===updated.id?updated:o));
   const found=updated.directions.find(d=>d.status==="ACTIVE"&&d.name===newDirectionName);
   if(found)setDirectionID(found.id);
  }catch(e){
   setDirectionMutationUncertain(true);
   setProductStepError("Создание направления не подтверждено. Проверьте список направлений перед повтором: "+String(e));
  }finally{setProductBusy(false)}
 }
 async function reloadAcademyOrganization(){
  try{
   const data=await api<{organizations:AcademyOrg[]}>("/v1/organizations");
   setOrgs(data.organizations);setOrgMutationUncertain(false);setDirectionMutationUncertain(false);
   const selected=data.organizations.find(o=>o.id===orgID)||data.organizations[0];
   if(selected){setOrgID(selected.id);await loadOrgProducts(selected.id)}
   else{setOrgID("");setOrgProducts([])}
   setProductMutationUncertain(false);setProductStepError("");
  }catch(e){setProductStepError("Не удалось проверить сохранённое состояние: "+String(e))}
 }
 async function prepareCommerceProduct(event:FormEvent){
  event.preventDefault();
  if(productMutationUncertain||!orgID||!directionID||!title.trim()||!commercialRef.trim()||!beneficiaryRef.trim()||!authorRefs.trim())return;
  setProductBusy(true);setProductStepError("");
  try{
   // On ambiguous errors never automatically retry this non-idempotent POST.
   // Reload canonical products so the author can recover a persisted draft.
   const created=await api<AcademyOrgProduct>(`/v1/organizations/${orgID}/products`,{
    name:title, direction_id:directionID, commercial_owner_ref:commercialRef.trim(),
    author_refs:authorRefs.split(",").map(x=>x.trim()).filter(Boolean),
    revenue_beneficiary_ref:beneficiaryRef.trim()
   });
   setPreparedProductID(created.id);
   await loadOrgProducts(orgID);
  }catch(e){
   setProductMutationUncertain(true);
   setProductStepError("Создание продукта не подтверждено. Обновите список продуктов и выберите существующий черновик прежде чем повторять: "+(e instanceof Error?e.message:String(e)));
   try{await loadOrgProducts(orgID)}catch{/* Preserve original error */ }
  }finally{setProductBusy(false)}
 }
 async function publishPreparedProduct(){
  if(!orgID||!preparedProductID)return;
  setProductBusy(true);setProductStepError("");
  try{
   const found=orgProducts.find(p=>p.id===preparedProductID);
   if(found?.status!=="PUBLISHED"){
    try{await api(`/v1/organizations/${orgID}/products/${preparedProductID}/publish`,{})}
    catch(e){
     // Publication may have committed even when the response was lost.
     await loadOrgProducts(orgID);
     setProductStepError("Публикация не подтверждена. Проверьте статус продукта и повторите только если он остался черновиком: "+(e instanceof Error?e.message:String(e)));
     return;
    }
   }
   await loadOrgProducts(orgID);
   await refresh();
   setProductID(preparedProductID);
   setNotice("Продукт опубликован. Теперь создайте курс.");
  }catch(e){setProductStepError(e instanceof Error?e.message:String(e))}
  finally{setProductBusy(false)}
 }
 async function createCourse(event:FormEvent){
  event.preventDefault();
  await act(()=>api<Course>("/v1/academy/drafts",
   {title:title.trim(),description}),async c=>{
   setNotice("Черновик курса создан.");await loadOutline(c);
  });
 }
 async function addModule(event:FormEvent){
  event.preventDefault();if(!course)return;
  await act(()=>api<Module>(
   `/v1/academy/courses/${course.id}/versions/${course.version_id}/modules`,
   {title:moduleTitle,ordinal:modules.length+1}),async()=>{setModuleTitle("");await loadOutline(course)});
 }
 async function addLesson(event:FormEvent){
  event.preventDefault();if(!course||!moduleID)return;
  const existing=lessons.filter(x=>x.module_id===moduleID);
  await act(()=>api<Lesson>(
   `/v1/academy/courses/${course.id}/versions/${course.version_id}/modules/${moduleID}/lessons`,
   {title:lessonTitle,ordinal:existing.length+1,kind:lessonKind,
    body:lessonKind==="READING"?lessonBody:"",
    media_asset_ref:lessonKind==="VIDEO"?lessonBody:"",
    passing_score:lessonKind==="ASSESSMENT"?passingScore:0,
    rubric_version:lessonKind==="ASSESSMENT"?"human-review-v1":""}),
   async()=>{setLessonTitle("");setLessonBody("");await loadOutline(course)});
 }
 return <main className="site-shell">
  <header className="site-header"><a className="brand" href="/">APGIC</a>
   <nav className="top-nav" aria-label="Разделы"><a href="/">Найти специалиста</a><a href="/organization">Организация</a><a href="/specialist">Специалист</a></nav>
  </header>
  <section className="how-section" style={{paddingTop:36}}>
   <p className="section-kicker">APGIC Academy</p>
   <h1>Обучение и курсы</h1>
   <p>Обучение имеет собственные версии, историю прохождения и проверяемые сертификаты.
   Сертификат не присваивает статус проверенного специалиста.</p>
   <p role="status" aria-live="polite">{notice}</p>
   <div className="how-grid">
    <section className="panel"><h2>Каталог</h2>
     {catalog.length===0?<p>Опубликованных курсов пока нет.</p>:catalog.map(c=>
      <article key={c.id}><h3>{c.title}</h3><p>{c.description}</p>
       <p>Версия {c.version_no}</p>
       {verifiedCourseIDs.includes(c.id)?<button disabled={busy} onClick={()=>void act(
        ()=>api<Enrollment>(`/v1/academy/courses/${c.id}/enroll`,{}),
        async e=>{setNotice("Подтверждено право доступа.");await loadLearning(e)})}>Начать обучение</button>:
        <p>Для записи нужна подтверждённая покупка курса. Эта страница не принимает платежи и не выдаёт доступ без Commerce/Entitlement.</p>}
      </article>)}
     <p>Доступ предоставляется только по подтверждённому entitlement. Если покупки нет, платформа не создаёт фиктивную оплату.</p>
    </section>
    <section className="panel"><h2>Мои обучения</h2>
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
     {enrollments.length===0?<p>Пока нет зачислений.</p>:enrollments.map(e=>
      <div key={e.id}><p>{e.course_title||"Курс"} · версия {e.version_no??"—"} · {e.state} · доступ {e.access_state??"неизвестен"}</p>
       <button disabled={busy||e.access_state!=="ACTIVE"} onClick={()=>void act(
        ()=>api<{lessons:Lesson[]}>(`/v1/academy/enrollments/${e.id}/learn`),
        async()=>{await loadLearning(e)})}>Продолжить</button>
      </div>)}
    </section>
    <form id="academy-author" className="panel specialist-form" onSubmit={createCourse}>
     <h2>Создать курс</h2>
     <p>Сначала содержание. Продажи и платежи можно настроить позже, когда курс будет готов.</p>
     <label htmlFor="academy-title">Название курса</label>
     <input id="academy-title" value={title} onChange={e=>setTitle(e.target.value)} required minLength={3} maxLength={200} placeholder="Например: Гипноз с нуля"/>
     <label htmlFor="academy-description">Описание (необязательно)</label>
     <textarea id="academy-description" value={description} onChange={e=>setDescription(e.target.value)}/>
     <button disabled={busy||title.trim().length<3}>Создать курс</button>
    </form>
   </div>
  </section>
  <section className="how-section">
   <h2>Мои курсы как автора</h2>
   <div className="how-grid">
    {mine.map(c=><article className="panel" key={c.id}>
     <h3>{c.title}</h3><p>Версия {c.version_no} · {c.state}</p>
     <button disabled={busy} onClick={()=>void act(
      ()=>api<{modules:Module[];lessons:Lesson[];state:string}>(`/v1/academy/courses/${c.id}/versions/${c.version_id}/outline`),
      async()=>loadOutline(c))}>Редактирование и содержание</button>
     <button disabled={busy} onClick={()=>void act(
      ()=>api<Course>(`/v1/academy/courses/${c.id}/versions`,{}),
      async d=>loadOutline({...c,version_id:d.version_id,version_no:d.version_no}))}>Новая версия</button>
    </article>)}
   </div>
  </section>
  {course&&<section className="how-section">
   <h2>Редактор: {course.title} — версия {course.version_no}</h2>
   {course.state==="DRAFT"&&<CloudMediaEditor courseId={course.id} versionId={course.version_id} lessons={lessons.map(l=>({id:l.id,title:l.title,kind:l.kind}))}/>}
   <div className="how-grid">
    <form className="panel specialist-form" onSubmit={addModule}>
     <h3>Модуль</h3><label htmlFor="academy-module">Название</label>
     <input id="academy-module" value={moduleTitle} onChange={e=>setModuleTitle(e.target.value)} required/>
     <button disabled={busy}>Добавить модуль</button>
    </form>
    <form className="panel specialist-form" onSubmit={addLesson}>
     <h3>Урок</h3><label htmlFor="academy-module-select">Модуль</label>
     <select id="academy-module-select" value={moduleID} onChange={e=>setModuleID(e.target.value)} required>
      {modules.map(m=><option key={m.id} value={m.id}>{m.ordinal}. {m.title}</option>)}
     </select>
     <label htmlFor="academy-lesson-title">Название</label>
     <input id="academy-lesson-title" value={lessonTitle} onChange={e=>setLessonTitle(e.target.value)} required/>
     <label htmlFor="academy-lesson-kind">Тип активности</label>
     <select id="academy-lesson-kind" value={lessonKind} onChange={e=>setLessonKind(e.target.value as Lesson["kind"])}>
      <option value="READING">Текстовый урок</option><option value="ASSESSMENT">Проверочное задание</option>
      <option value="VIDEO">Видеоматериал по media asset reference</option>
     </select>
     <label htmlFor="academy-lesson-body">Материал или идентификатор медиаресурса</label>
     <textarea id="academy-lesson-body" value={lessonBody} onChange={e=>setLessonBody(e.target.value)}/>
     {lessonKind==="ASSESSMENT"&&<><label htmlFor="academy-pass">Проходной балл</label>
      <input id="academy-pass" type="number" min={1} max={100} value={passingScore} onChange={e=>setPassingScore(Number(e.target.value))}/></>}
     <button disabled={busy||!moduleID}>Добавить урок</button>
    </form>
    <section className="panel"><h3>Содержание</h3>
     {modules.map(m=><article key={m.id}><h4>{m.ordinal}. {m.title}</h4>
      <ul>{lessons.filter(l=>l.module_id===m.id).map(l=><li key={l.id}>{l.title} · {l.kind}</li>)}</ul>
     </article>)}
     {course.state==="DRAFT"&&<section aria-label="Расписание и лимиты уроков">
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
     </section>}
     <section aria-label="Декларация авторских прав">
      <h4>Авторские права на материалы курса</h4>
      <p>Подтвердите авторство оригинального содержания. Сторонняя лицензия требует отдельной проверки.</p>
      <label htmlFor="academy-original-rights">Источник оригинальных материалов</label>
      <input id="academy-original-rights" value={rightsSource}
       onChange={e=>{setRightsSource(e.target.value);setRightsRecorded(false)}}/>
      <label><input type="checkbox" checked={rightsAgreed} onChange={e=>setRightsAgreed(e.target.checked)}/>
       Подтверждаю авторство и право публикации этих оригинальных материалов</label>
      <button type="button" disabled={busy||!rightsAgreed||rightsSource.trim().length<12||lessons.length===0}
       onClick={()=>void act(()=>api<{id:string}>(
        `/v1/academy/courses/${course.id}/versions/${course.version_id}/rights/original`,
        {provenance_ref:rightsSource,acknowledged:true}),()=>setRightsRecorded(true))}>
       Зафиксировать права на текущую версию</button>
      {rightsRecorded&&<p>Подтверждение зафиксировано. Изменения уроков потребуют нового подтверждения.</p>}
     </section>
     <button disabled={busy||lessons.length===0} onClick={()=>void act(
      ()=>api<Course>(`/v1/academy/courses/${course.id}/versions/${course.version_id}/publish`,{}),
      async()=>{setNotice("Версия опубликована и больше не редактируется.");setCourse(null)})}>Опубликовать версию</button>
    </section>
   </div>
  </section>}
  {course&&course.state==="PUBLISHED"&&<section className="how-section">
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
  {activeEnrollment&&<section className="how-section">
   <h2>Прохождение курса</h2>
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
  {authorReport&&<section className="how-section" aria-label="Результаты обучения и подтверждённые оплаты">
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
  <section id="academy-reviews" className="how-section"><h2>Проверка заданий преподавателем</h2>
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
    <button disabled={busy}>Зафиксировать оценку</button>
   </form>
  </section>
 </main>;
}
