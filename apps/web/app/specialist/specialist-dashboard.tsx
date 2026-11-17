"use client";
import {useState} from "react";
import {SpecialistOnboarding,type SpecialistOnboardingSection} from "./onboarding";
import {SpecialistWorkspaceCalendar} from "./workspace-calendar";
import {SpecialistAcademyWorkspace} from "./academy-workspace";
import {SpecialistConsultationAccess} from "./consultation-access";
import "./specialist-dashboard.css";

type Section=SpecialistOnboardingSection|"calendar"|"consultations"|"academy";
const sections:Array<{id:Section;title:string;symbol:string;hint:string}>=[
 {id:"profile",title:"Профиль",symbol:"◯",hint:"Имя и профессия"},
 {id:"about",title:"О себе",symbol:"✎",hint:"Описание и опыт"},
 {id:"topics",title:"Направления",symbol:"◇",hint:"С чем работаете"},
 {id:"documents",title:"Документы",symbol:"▤",hint:"Подтверждение"},
 {id:"publication",title:"Публикация",symbol:"✓",hint:"Статус и проверка"},
 {id:"offers",title:"Услуги и цены",symbol:"▦",hint:"Продукты и время"},
 {id:"calendar",title:"Расписание",symbol:"▦",hint:"Записи и слоты"},
 {id:"consultations",title:"Консультации",symbol:"▷",hint:"Рабочее место"},
 {id:"academy",title:"Курсы и ученики",symbol:"◈",hint:"Академия"}
];
const onboardingSections:SpecialistOnboardingSection[]=["profile","about","topics","documents","publication","offers"];
export function SpecialistDashboard(){
 const [section,setSection]=useState<Section>("profile");
 const current=sections.find(item=>item.id===section)!;
 function navigate(next:Section){setSection(next);window.scrollTo({top:0,behavior:"instant"});}
 return <main className="specialist-dashboard">
  <aside className="specialist-dashboard-sidebar" aria-label="Разделы специалиста">
   <a className="specialist-dashboard-brand" href="/" aria-label="APGIC — главная">
    <span className="specialist-dashboard-mark">A</span><span>APGIC<small>Для специалистов</small></span>
   </a>
   <nav aria-label="Навигация кабинета специалиста">
    {sections.map(item=><button key={item.id} type="button" aria-current={section===item.id?"page":undefined}
      onClick={()=>navigate(item.id)} className={section===item.id?"specialist-dashboard-selected":""}>
     <span aria-hidden="true" className="specialist-dashboard-icon">{item.symbol}</span>
     <span>{item.title}</span>
    </button>)}
   </nav>
   <div className="specialist-dashboard-links">
    <a href="/personal">Личный кабинет</a>
    <a href="/organization">Организация</a>
    <a href="/specialists">Каталог специалистов</a>
    <a href="/specialist/now">Принимать сейчас</a>
    <a href="/specialist/growth">Привлечение клиентов</a>
   </div>
  </aside>
  <div className="specialist-dashboard-body">
   <header className="specialist-dashboard-topbar">
    <div><p>APGIC / Специалист</p><h1 className="specialist-dashboard-app-title">Кабинет специалиста APGIC</h1></div>
    <a href="/specialists" className="specialist-dashboard-secondary">Каталог специалистов ↗</a>
   </header>
   <div className="specialist-dashboard-intro">
    <p className="specialist-dashboard-kicker">Ваше пространство</p>
    <h2>{current.title}</h2>
    <p>{current.hint}. Изменения и статусы подтверждаются APGIC.</p>
   </div>
   <div className="specialist-dashboard-progress" role="group" aria-label="Быстрые шаги профиля">
    {onboardingSections.map((key,index)=><button type="button" key={key}
     aria-current={section===key?"step":undefined} onClick={()=>navigate(key)}>
      <span>{index+1}</span><strong>{sections.find(item=>item.id===key)?.title}</strong>
     </button>)}
   </div>
   <div className="specialist-dashboard-surface">
    <div className="specialist-dashboard-main">
     <div hidden={!onboardingSections.includes(section as SpecialistOnboardingSection)}>
      <SpecialistOnboarding activeSection={onboardingSections.includes(section as SpecialistOnboardingSection)?section as SpecialistOnboardingSection:"profile"} onNavigate={navigate}/>
     </div>
     {section==="calendar"&&<SpecialistWorkspaceCalendar/>}
     {section==="consultations"&&<section id="specialist-consultations"><SpecialistConsultationAccess/></section>}
     {section==="academy"&&<SpecialistAcademyWorkspace/>}
    </div>
    <aside className="specialist-dashboard-help" aria-label="Полезные действия">
     <h2>Что важно знать</h2>
     {section==="profile"?<p>Сначала сохраните имя и профессию. После этого станут доступны остальные этапы.</p>:
      section==="documents"?<p>Подтверждающий документ отправляется на независимую проверку. Сам факт отправки не меняет статус квалификации.</p>:
      section==="publication"?<p>Публикация разрешается только после проверки профиля и компетенции. Неуспешная проверка не открывает платную запись.</p>:
      section==="offers"?<p>Цена привязывается к опубликованному продукту организации, а время — к действующему предложению.</p>:
      section==="calendar"?<p>Здесь отображаются записи и свободное время из рабочего календаря APGIC.</p>:
      section==="consultations"?<p>Доступ к конкретной консультации проверяется сервером по идентификатору записи.</p>:
      section==="academy"?<p>Курсы, ученики и проверки работают через существующую Academy. Второй аккаунт не требуется.</p>:
      <p>Данные анкеты становятся публичными только в соответствии с правилами проверки и публикации.</p>}
     <div className="specialist-dashboard-help-links">
      <button type="button" onClick={()=>navigate("publication")}>Проверка и публикация →</button>
      <button type="button" onClick={()=>navigate("calendar")}>Мои записи →</button>
      <a href="/academy">Открыть Academy ↗</a>
     </div>
    </aside>
   </div>
  </div>
 </main>;
}
