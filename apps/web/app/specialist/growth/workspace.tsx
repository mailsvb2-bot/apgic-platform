"use client";
import {useEffect,useState} from "react";
import {growthURL} from "../../../src/growth-client";
type Offer={id:string;topic_id:string;state:string;price_minor:number;currency:string};
type Report={link_id:string;offer_id:string;source:string;state:string;touches:number;help_intents:number;confirmed_intents:number};
type Link={path:string;id:string};
async function api<T>(url:string,init?:RequestInit):Promise<T>{
 const r=await fetch(url,{...init,credentials:"include",cache:"no-store"});
 const v=await r.json() as T & {message_safe?:string};
 if(!r.ok)throw Error(v.message_safe||"Сервис временно недоступен.");
 return v;
}
export function GrowthWorkspace(){
 const [offers,setOffers]=useState<Offer[]>([]);
 const [reports,setReports]=useState<Report[]>([]);
 const [url,setURL]=useState("");
 const [error,setError]=useState("");
 const [loading,setLoading]=useState(true);
 const [busy,setBusy]=useState(false);
 const load=async()=>{
  const [s,r]=await Promise.all([
   api<{offers:Offer[]}>("/v1/specialist/offers"),
   api<{items:Report[]}>("/v1/specialist/growth-report"),
  ]);
  setOffers((s.offers||[]).filter(x=>x.state==="ACTIVE"));setReports(r.items||[]);
 };
 useEffect(()=>{let alive=true;
  void Promise.all([
   api<{offers:Offer[]}>("/v1/specialist/offers"),
   api<{items:Report[]}>("/v1/specialist/growth-report"),
  ]).then(([s,r])=>{if(alive){setOffers((s.offers||[]).filter(x=>x.state==="ACTIVE"));setReports(r.items||[]);setLoading(false);}})
   .catch(e=>{if(alive){setError(e instanceof Error?e.message:"Недоступно");setLoading(false);}});
  return()=>{alive=false;};
 },[]);
 const issue=async(id:string)=>{
  setBusy(true);setError("");
  try{
   const link=await api<Link>("/v1/specialist/growth-links",{
    method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify({offer_id:id}),
   });
   setURL(growthURL(link.path,window.location.origin));await load();
  }catch(e){setError(e instanceof Error?e.message:"Ошибка ссылки");}finally{setBusy(false);}
 };
 const revoke=async(id:string)=>{
  setBusy(true);setError("");
  try{await api("/v1/specialist/growth-links/"+encodeURIComponent(id)+"/revoke",{method:"POST"});await load();}
  catch(e){setError(e instanceof Error?e.message:"Не удалось отозвать");}finally{setBusy(false);}
 };
 return <div>
  {error&&<p role="alert">{error}</p>}
  {loading&&<p role="status">Загружаем предложения…</p>}
  {!loading&&offers.length===0&&<p>Действующих платных предложений пока нет. Сначала подтвердите квалификацию и опубликуйте предложение.</p>}
  <div className="how-grid">{offers.map(o=><article key={o.id}>
   <h2>{o.topic_id}</h2><p>Цена: {new Intl.NumberFormat("ru-RU").format(o.price_minor/100)} {o.currency}</p>
   <button type="button" disabled={busy} onClick={()=>void issue(o.id)}>Создать ссылку</button>
  </article>)}</div>
  {url&&<div role="status"><p>Поделитесь ссылкой через разрешённый канал:</p>
   <p><a href={url} rel="noreferrer">{url}</a></p>
   <button type="button" onClick={()=>void navigator.clipboard.writeText(url)}>Скопировать</button>
  </div>}
  <section aria-label="Атрибуция"><h2>Результаты</h2>
   <p>Учитываются только переходы и связанные запросы. Оплата и прибыль здесь пока не подсчитываются — неподтверждённые продажи не показываются.</p>
   {reports.map(x=><article key={x.link_id}>
    <p>{x.source} · {x.state} · предложение {x.offer_id}</p>
    <p>Переходов: {x.touches}. Запросов: {x.help_intents}. Подтверждено: {x.confirmed_intents}.</p>
    {x.state==="ACTIVE"&&<button type="button" disabled={busy} onClick={()=>void revoke(x.link_id)}>Отозвать ссылку</button>}
   </article>)}
  </section>
 </div>;
}
