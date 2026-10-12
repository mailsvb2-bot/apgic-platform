import { NowJourney } from "./now-journey";
export const dynamic = "force-dynamic";
export const metadata = {title:"Поговорить со специалистом сейчас — APGIC"};
export default function NowPage(){
 return <main className="site-shell" style={{maxWidth:900,padding:"2rem 1rem",margin:"auto"}}>
  <nav aria-label="Навигация"><a href="/">← На главную</a></nav>
  <p className="eyebrow">APGIC · Duty/Now</p>
  <h1>Поговорить со специалистом сейчас</h1>
  <p className="hero-lead">Проверим запрос и попробуем связаться с дежурным специалистом. Принятие предложения не означает подтверждённую запись: оплата и доступ к разговору разрешаются отдельно.</p>
  <NowJourney/>
  <p style={{marginTop:28}}>Если никто не ответил, <a href="/#start">можно выбрать специалиста и время обычной записи</a>.</p>
 </main>;
}