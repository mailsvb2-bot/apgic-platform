"use client";
import {useEffect,useState} from "react";
import {growthTarget,type GrowthTouch} from "../../../src/growth-client";
export function GrowthLanding({token}:{token:string}){
 const [result,setResult]=useState<GrowthTouch|null>(null);
 const [status,setStatus]=useState<"loading"|"success"|"invalid">("loading");
 useEffect(()=>{
  let active=true;
  if(!/^g1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(token)){setStatus("invalid");return()=>{active=false;};}
  void fetch("/v1/growth/touches",{
   method:"POST",credentials:"include",headers:{"content-type":"application/json"},
   body:JSON.stringify({token}),cache:"no-store",referrerPolicy:"no-referrer",
  }).then(async r=>{if(!r.ok)throw Error("UNAVAILABLE");return r.json() as Promise<GrowthTouch>;})
    .then(v=>{if(active){setResult(v);setStatus("success");}})
    .catch(()=>{if(active)setStatus("invalid");});
  return()=>{active=false;};
 },[token]);
 return <main className="site-shell">
  <header className="site-header"><a className="brand" href="/">APGIC</a></header>
  <section className="specialist-hero"><div>
   <p className="eyebrow">Защищённое приглашение</p><h1>Подбор специалиста</h1>
   {status==="loading"&&<p role="status">Проверяем предложение…</p>}
   {status==="invalid"&&<div role="alert"><p>Ссылка устарела, отозвана или специалист больше недоступен.</p><a className="button-link primary-action" href="/#start">Перейти к подбору</a></div>}
   {status==="success"&&result&&<div>
    <p>Приглашение специалиста: <strong>{result.specialist_name}</strong>.</p>
    <p>Квалификация и доступность повторно проверяются при записи. Эта ссылка не гарантирует конкретный слот.</p>
    <a className="button-link primary-action" href={growthTarget(result.topic_id)}>Продолжить подбор и запись</a>
   </div>}
  </div></section>
  <footer className="site-footer"><p>Статистика переходов не раскрывает специалисту ваши личные запросы и содержание консультаций.</p></footer>
 </main>;
}
