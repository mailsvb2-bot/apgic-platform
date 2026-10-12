import type {Metadata} from "next";
import {notFound} from "next/navigation";

export const dynamic="force-dynamic";
type Offer={offer_id:string; specialist_id:string; display_name:string; profession:string; price_minor:number;currency:string;has_available_slot:boolean};
type PublicTopic={topic:{slug:string;topic_code:string;title:string;description:string}; offers:Offer[];indexable:boolean;publication_policy_version:string};

async function publicTopic(slug:string):Promise<PublicTopic|null> {
 if (!/^[a-z]{2,24}$/.test(slug))return null;
 const origin=process.env.APGIC_API_ORIGIN||"http://127.0.0.1:43111";
 try {
   const url=new URL("/v1/public/help/"+encodeURIComponent(slug),origin);
   const r=await fetch(url,{cache:"no-store",headers:{accept:"application/json"}});
   if(!r.ok)return null;
   const p=await r.json() as PublicTopic;
   if(!p.topic||p.topic.slug!==slug||!Array.isArray(p.offers)||typeof p.indexable!=="boolean")return null;
   return p;
 }catch{return null;}
}

export async function generateMetadata({params}:{params:Promise<{slug:string}>}):Promise<Metadata>{
 const {slug}=await params;
 const p=await publicTopic(slug);
 if(!p)return {title:"Подбор специалистов — APGIC",robots:{index:false,follow:false}};
 return {
  title:p.topic.title+" — APGIC",
  description:p.topic.description,
  alternates:{canonical:"https://apgic.ru/help/"+p.topic.slug},
  robots:{index:p.indexable,follow:p.indexable},
  openGraph:{title:p.topic.title,description:p.topic.description,
    url:"https://apgic.ru/help/"+p.topic.slug,type:"website"},
 };
}

export default async function HelpPage({params}:{params:Promise<{slug:string}>}){
 const {slug}=await params;
 if(!/^[a-z]{2,24}$/.test(slug))notFound();
 const p=await publicTopic(slug);
 return <main className="site-shell">
  <header className="site-header" aria-label="Навигация">
   <a href="/" className="brand"><span className="brand-mark" aria-hidden="true">A</span> APGIC</a>
   <nav className="top-nav"><a href="/">Главная</a><a href="/specialist">Для специалистов</a></nav>
  </header>
  <section className="specialist-hero"><div>
   <p className="eyebrow">Проверенные предложения · APGIC</p>
   <h1>{p?.topic.title||"Поиск специалиста"}</h1>
   <p className="hero-lead">{p?.topic.description||"Сейчас не удалось получить актуальные данные о специалистах."}</p>
   {p&&<a className="button-link primary-action"
    href={"/?topic="+encodeURIComponent(p.topic.topic_code)+"#start"}>Подобрать специалиста и время</a>}
   {!p&&<a className="button-link primary-action" href="/#start">Начать общий подбор</a>}
  </div></section>
  {p&&<section className="how-section" aria-label="Актуальные предложения">
   <h2>Специалисты и предложения</h2>
   {p.offers.length===0?<p>Подходящих опубликованных предложений сейчас нет. Вы можете описать запрос для обычного подбора.</p>:null}
   <div className="how-grid">{p.offers.map(o=><article key={o.offer_id}>
     <h3>{o.display_name}</h3><p>{o.profession}</p>
     <p>Цена: {new Intl.NumberFormat("ru-RU").format(o.price_minor/100)} {o.currency}</p>
     <p>{o.has_available_slot?"Есть актуальные слоты для выбора":"Свободное время пока не подтверждено"}</p>
   </article>)}</div>
   {!p.indexable&&<p>Для широкого публичного размещения пока недостаточно подтверждённых доступных предложений. APGIC не обещает запись там, где она не подтверждена.</p>}
  </section>}
  <footer className="site-footer"><p>APGIC не выдаёт диагнозы и не гарантирует результаты консультаций.</p></footer>
 </main>;
}
