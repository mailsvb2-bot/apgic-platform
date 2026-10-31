"use client";

import {useCallback,useEffect,useState} from "react";

type Course={id:string;title:string;state:string;version_no:number};
type Report={courses:Array<{course_id:string;title:string;enrollments:number;completed:number;pending_assessments:number;passed_assessments:number}>;revenue_state:string};

async function getAcademy<T>(path:string):Promise<T>{
 const response=await fetch(path,{credentials:"same-origin",cache:"no-store",headers:{"Accept":"application/json"}});
 if(!response.ok)throw new Error(response.status===401?"Войдите в свой аккаунт APGIC":"Не удалось загрузить данные обучения");
 return response.json() as Promise<T>;
}

// One specialist workspace entry-point, backed by the canonical Academy API.
// Course editing, grading, identity, entitlements and finance remain with their
// existing owners; this is a read-only navigation and status surface.
export function SpecialistAcademyWorkspace(){
 const [courses,setCourses]=useState<Course[]>([]);
 const [report,setReport]=useState<Report|null>(null);
 const [notice,setNotice]=useState("");
 const [busy,setBusy]=useState(false);
 const refresh=useCallback(async()=>{
  setBusy(true);
  try{
   const [owned,summary]=await Promise.all([
    getAcademy<{courses:Course[]}>("/v1/academy/my-courses"),
    getAcademy<Report>("/v1/academy/author/report")
   ]);
   setCourses(owned.courses);
   setReport(summary);
   setNotice("");
  }catch(error){setNotice(error instanceof Error?error.message:"Недоступен сервис Academy")}
  finally{setBusy(false)}
 },[]);
 useEffect(()=>{void refresh()},[refresh]);
 const pending=report?.courses.reduce((sum,c)=>sum+c.pending_assessments,0)??0;
 return <section className="how-section" id="specialist-academy" aria-labelledby="specialist-academy-title">
  <p className="section-kicker">Academy · Преподавание</p>
  <h2 id="specialist-academy-title">Мои курсы и ученики</h2>
  <p>Тот же аккаунт специалиста: создавайте программы, публикуйте версии, принимайте учеников и проверяйте работы. Финансовые права и обучение управляются каноническими сервисами APGIC.</p>
  <p role="status" aria-live="polite">{notice}</p>
  <div className="hero-actions">
   <a className="button-link primary-action" href="/academy">Создать или редактировать курс</a>
   <a className="button-link secondary-action" href="/academy">Проверить работы {pending>0?`(${pending})`:""}</a>
   <button type="button" onClick={()=>void refresh()} disabled={busy}>Обновить данные</button>
  </div>
  <div className="how-grid specialist-grid">
   <article className="panel"><h3>Мои курсы</h3><p>{courses.length} в кабинете</p>
    {courses.map(c=><p key={c.id}>{c.title} · версия {c.version_no} · {c.state}</p>)}
    {courses.length===0&&<p>Курсы пока не созданы.</p>}
   </article>
   <article className="panel"><h3>Мои ученики</h3>
    <p>Зачислено: {report?.courses.reduce((s,c)=>s+c.enrollments,0)??"—"}</p>
    <p>Завершили: {report?.courses.reduce((s,c)=>s+c.completed,0)??"—"}</p>
   </article>
   <article className="panel"><h3>Задания и доходы</h3>
    <p>Ожидают проверки: {report?pending:"—"}</p>
    <p>Доход показывается только после канонической финансовой сверки; подтверждённые списания не равны выплате специалисту.</p>
   </article>
  </div>
 </section>;
}
