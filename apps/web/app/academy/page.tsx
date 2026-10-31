"use client";

import {useCallback,useEffect,useState,type FormEvent} from "react";

type Course = {id:string;product_id:string;title:string;description:string;state:string;version_id:string;version_no:number};
type Module = {id:string;version_id:string;title:string;ordinal:number};
type Lesson = {id:string;module_id:string;title:string;kind:"READING"|"VIDEO"|"ASSESSMENT";ordinal:number;body?:string;media_asset_ref?:string;completed?:boolean;passing_score?:number};
type Enrollment = {id:string;version_id:string;course_id?:string;course_title?:string;version_no?:number;state:string;access_state?:"ACTIVE"|"REVOKED"};
type Certificate = {verification_url:string;state:string;evidence_candidate_state:string};
type Cohort = {id:string;version_id:string;label:string;capacity:number;members:number;starts_at?:string;enrollment_closes_at?:string};
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

export default function AcademyPage(){
 const [catalog,setCatalog]=useState<Course[]>([]);
 const [mine,setMine]=useState<Course[]>([]);
 const [enrollments,setEnrollments]=useState<Enrollment[]>([]);
 const [course,setCourse]=useState<Course|null>(null);
 const [modules,setModules]=useState<Module[]>([]);
 const [lessons,setLessons]=useState<Lesson[]>([]);
 const [activeEnrollment,setActiveEnrollment]=useState<Enrollment|null>(null);
 const [certificate,setCertificate]=useState<Certificate|null>(null);
 const [cohorts,setCohorts]=useState<Cohort[]>([]);
 const [cohortTitle,setCohortTitle]=useState("");
 const [cohortCapacity,setCohortCapacity]=useState(20);
 const [cohortStarts,setCohortStarts]=useState("");
 const [cohortCloses,setCohortCloses]=useState("");
 const [notice,setNotice]=useState("");
 const [busy,setBusy]=useState(false);
 const [productID,setProductID]=useState("");
 const [title,setTitle]=useState("");
 const [description,setDescription]=useState("");
 const [moduleTitle,setModuleTitle]=useState("");
 const [lessonTitle,setLessonTitle]=useState("");
 const [lessonBody,setLessonBody]=useState("");
 const [lessonKind,setLessonKind]=useState<Lesson["kind"]>("READING");
 const [moduleID,setModuleID]=useState("");
 const [passingScore,setPassingScore]=useState(80);
 const [attemptResponse,setAttemptResponse]=useState("");
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
  const [owned,learning,reviews,report]=await Promise.allSettled([
   api<{courses:Course[]}>("/v1/academy/my-courses"),
   api<{enrollments:Enrollment[]}>("/v1/academy/my-enrollments"),
   api<{attempts:ReviewAttempt[]}>("/v1/academy/review/attempts"),
   api<AuthorReport>("/v1/academy/author/report")
  ]);
  if(owned.status==="fulfilled")setMine(owned.value.courses);
  if(learning.status==="fulfilled")setEnrollments(learning.value.enrollments);
  if(reviews.status==="fulfilled")setPendingReviews(reviews.value.attempts);
  if(report.status==="fulfilled")setAuthorReport(report.value);
 },[]);
 useEffect(()=>{void refresh().catch(error=>setNotice(String(error)))},[refresh]);

 async function act<T>(work:()=>Promise<T>,after:(result:T)=>Promise<void>|void){
  setBusy(true);setNotice("");
  try{const result=await work();await after(result);await refresh()}
  catch(error){setNotice(error instanceof Error?error.message:"Операция не выполнена")}
  finally{setBusy(false)}
 }
 async function loadOutline(selected:Course){
  setCourse(selected);setActiveEnrollment(null);setCertificate(null);
  const d=await api<{state:string;modules:Module[];lessons:Lesson[]}>(
   `/v1/academy/courses/${selected.id}/versions/${selected.version_id}/outline`);
  setModules(d.modules);setLessons(d.lessons);setModuleID(d.modules[0]?.id||"");
 }
 async function loadLearning(item:Enrollment){
  const available=await api<{cohorts:Cohort[]}>(`/v1/academy/enrollments/${item.id}/cohorts`);
  setCohorts(available.cohorts);
  setActiveEnrollment(item);setCourse(null);setCertificate(null);
  const d=await api<{lessons:Lesson[]}>(`/v1/academy/enrollments/${item.id}/learn`);
  setLessons(d.lessons);setModules([]);
 }
 async function createCourse(event:FormEvent){
  event.preventDefault();
  await act(()=>api<Course>("/v1/academy/courses",
   {product_id:productID,title,description}),async c=>{
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
       <button disabled={busy} onClick={()=>void act(
        ()=>api<Enrollment>(`/v1/academy/courses/${c.id}/enroll`,{}),
        async e=>{setNotice("Подтверждено право доступа.");await loadLearning(e)})}>Начать обучение</button>
      </article>)}
     <p>Доступ предоставляется только по подтверждённому entitlement. Если покупки нет, платформа не создаёт фиктивную оплату.</p>
    </section>
    <section className="panel"><h2>Мои обучения</h2>
     {enrollments.length===0?<p>Пока нет зачислений.</p>:enrollments.map(e=>
      <div key={e.id}><p>{e.course_title||"Курс"} · версия {e.version_no??"—"} · {e.state} · доступ {e.access_state??"неизвестен"}</p>
       <button disabled={busy||e.access_state!=="ACTIVE"} onClick={()=>void act(
        ()=>api<{lessons:Lesson[]}>(`/v1/academy/enrollments/${e.id}/learn`),
        async()=>{await loadLearning(e)})}>Продолжить</button>
      </div>)}
    </section>
    <form className="panel specialist-form" onSubmit={createCourse}>
     <h2>Автору: создать курс</h2>
     <p>Сначала опубликуйте принадлежащий вам Product/Offer в APGIC. Курс сохраняет его юридического и коммерческого владельца.</p>
     <label htmlFor="academy-product">UUID опубликованного продукта</label>
     <input id="academy-product" value={productID} onChange={e=>setProductID(e.target.value)} required/>
     <label htmlFor="academy-title">Название</label>
     <input id="academy-title" value={title} onChange={e=>setTitle(e.target.value)} required minLength={3}/>
     <label htmlFor="academy-description">Описание</label>
     <textarea id="academy-description" value={description} onChange={e=>setDescription(e.target.value)}/>
     <button disabled={busy}>Создать черновик</button>
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
  </section>}
  {activeEnrollment&&<section className="how-section">
   <h2>Прохождение курса</h2>
   <h3>Доступные учебные потоки</h3>
   {cohorts.length===0?<p>Потоки для этой версии курса ещё не объявлены.</p>:cohorts.map(h=><article className="panel" key={h.id}>
    <strong>{h.label}</strong><p>Мест: {h.members} из {h.capacity}; начало: {h.starts_at?new Date(h.starts_at).toLocaleString("ru-RU"):"не задано"}</p>
    <button disabled={busy||h.members>=h.capacity} onClick={()=>void act(
     ()=>api(`/v1/academy/cohorts/${h.id}/join`,{enrollment_id:activeEnrollment.id}),
     async()=>{setNotice("Зачисление в поток подтверждено.");await loadLearning(activeEnrollment)})}>Записаться в поток</button>
   </article>)}
   <p>Материал доступен только владельцу активного права обучения.</p>
   <div className="how-grid">
    {lessons.map(l=><article className="panel" key={l.id}>
     <h3>{l.title}</h3><p>{l.kind} {l.completed?"· выполнено":""}</p>
     {l.kind==="READING"&&<p style={{whiteSpace:"pre-wrap"}}>{l.body}</p>}
     {l.kind==="VIDEO"&&<p>Видеодоступ выдаётся только через совместимый media provider с подтверждением playback.</p>}
     {l.kind==="ASSESSMENT"&&<form onSubmit={e=>{e.preventDefault();void act(
      ()=>api<{id:string}>(`/v1/academy/enrollments/${activeEnrollment.id}/assessments/${l.id}/attempts`,
      {response:attemptResponse}),async a=>{setNotice(`Работа отправлена на проверку. ID: ${a.id}`);setAttemptResponse("")})}}>
      <label htmlFor={`attempt-${l.id}`}>Ответ</label>
      <textarea id={`attempt-${l.id}`} value={attemptResponse} onChange={e=>setAttemptResponse(e.target.value)} required/>
      <button disabled={busy}>Отправить на проверку</button></form>}
     {l.kind==="READING"&&<button disabled={busy||l.completed} onClick={()=>void act(
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
  <section className="how-section"><h2>Проверка заданий преподавателем</h2>
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
