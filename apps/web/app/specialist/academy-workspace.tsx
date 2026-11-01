"use client";

import {useCallback,useEffect,useState} from "react";

type Course={id:string;title:string;state:string;version_no:number};
type Report={courses:Array<{course_id:string;title:string;enrollments:number;completed:number;pending_assessments:number;passed_assessments:number}>;gross_captures:Array<{course_id:string;currency:string;captured_count:number;captured_gross_minor:number;active_count:number;active_gross_minor:number}>;revenue_state:string};

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
   <a className="button-link primary-action" href="/academy#academy-author">Создать или редактировать курс</a>
   <a className="button-link secondary-action" href="/academy#academy-reviews">Проверить работы {pending>0?`(${pending})`:""}</a>
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
    {report?.gross_captures.length?report.gross_captures.map(item=>
     <p key={item.course_id+"-"+item.currency}>
      {courses.find(c=>c.id===item.course_id)?.title??"Курс"}: подтверждено оплат {item.captured_count};
      валовая сумма {item.captured_gross_minor} {item.currency} (минимальные денежные единицы).
      <br/>Из этих исторических оплат связаны с действующим зачислением и правом доступа: {item.active_count}, сумма исходных списаний {item.active_gross_minor} {item.currency}.
     </p>):<p>Подтверждённые оплаты курсов пока не отображаются.</p>}
    <p>Это исторические подтверждённые валовые списания Commerce и текущий статус прав доступа, а не доход к выплате. Разница между суммами не равна возвратам: ученик мог ещё не зачислиться, зачисление могло быть отозвано или срок доступа истёк. Чистый доход, возвраты, комиссии и расчёты с автором подтверждает только Finance.</p>
   </article>
  </div>
 </section>;
}
