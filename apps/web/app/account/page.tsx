"use client";

import {useState,type FormEvent} from "react";

type Challenge={pair_id:string;code:string;proof:string;expires_at:string};
async function post<T>(path:string,body?:unknown):Promise<T>{
 const response=await fetch(path,{
  method:"POST",credentials:"same-origin",cache:"no-store",
  ...(body===undefined?{}:{headers:{"Content-Type":"application/json"},body:JSON.stringify(body)})
 });
 const data=await response.json().catch(()=>({}));
 if(!response.ok)throw new Error(data.message_safe||data.code||"Не удалось выполнить операцию");
 return data as T;
}

export default function AccountPage(){
 const [challenge,setChallenge]=useState<Challenge|null>(null);
 const [code,setCode]=useState("");
 const [busy,setBusy]=useState(false);
 const [notice,setNotice]=useState("");
 async function start(){
  if(busy)return;
  setBusy(true);setNotice("");
  try{
   const started=await post<Challenge>("/v1/account/devices/pair/start");
   setChallenge(started);
   setNotice("На этом устройстве создан запрос. Подтвердите код на устройстве, где ваши курсы и данные уже доступны.");
  }catch(error){setNotice(String(error))}
  finally{setBusy(false)}
 }
 async function approve(event:FormEvent){
  event.preventDefault();
  if(busy||code.trim().length!==16)return;
  setBusy(true);setNotice("");
  try{
   await post("/v1/account/devices/pair/approve",{code:code.trim().toUpperCase()});
   setNotice("Подключение подтверждено. Вернитесь на второе устройство и нажмите «Завершить вход».");
   setCode("");
  }catch(error){setNotice(String(error))}
  finally{setBusy(false)}
 }
 async function claim(){
  if(busy||!challenge)return;
  setBusy(true);setNotice("");
  try{
   await post("/v1/account/devices/pair/claim",{pair_id:challenge.pair_id,proof:challenge.proof});
   // Never keep the claim secret after successful sign-in.
   setChallenge(null);
   window.location.assign("/");
  }catch(error){setNotice(String(error))}
  finally{setBusy(false)}
 }
 return <main className="site-shell">
  <header className="site-header"><a className="brand" href="/">APGIC</a>
   <nav className="top-nav"><a href="/academy">Обучение</a><a href="/organization">Организация</a><a href="/specialist">Специалист</a><a href="/personal">Личный кабинет</a></nav>
  </header>
  <section className="how-section">
   <h1>Один аккаунт на нескольких устройствах</h1>
   <p>Без передачи пароля или cookie. Ваши курсы, организации и личные данные остаются привязаны к одной подтверждённой учётной записи.</p>
   <div className="how-grid">
    <article className="panel">
     <h2>1. На новом телефоне или компьютере</h2>
     <p>Откройте эту страницу на устройстве, которое хотите подключить. Запрос действует 5 минут.</p>
     <button type="button" disabled={busy||challenge!==null} onClick={()=>void start()}>Получить одноразовый код</button>
     {challenge&&<div role="status">
      <p>Код для подтверждения на первом устройстве:</p>
      <p><strong style={{fontSize:"1.4rem",letterSpacing:".12em",overflowWrap:"anywhere"}}>{challenge.code}</strong></p>
      <p>Действителен до {new Date(challenge.expires_at).toLocaleTimeString("ru-RU")}. Не передавайте код незнакомым людям.</p>
      <button type="button" disabled={busy} onClick={()=>void claim()}>Завершить вход после подтверждения</button>
      <button type="button" disabled={busy} onClick={()=>{setChallenge(null);setNotice("Запрос больше не отображается. Его срок действия ограничен пятью минутами.");}}>Начать заново</button>
     </div>}
    </article>
    <article className="panel">
     <h2>2. На устройстве с существующим аккаунтом</h2>
     <p>Откройте APGIC здесь, убедитесь, что вы видите свои данные. Введите код со второго устройства и явно подтвердите подключение.</p>
     <form onSubmit={approve}>
      <label htmlFor="pair-code">Одноразовый код</label>
      <input id="pair-code" value={code} onChange={event=>setCode(event.target.value.toUpperCase().replace(/[^A-F0-9]/g,"").slice(0,16))} required maxLength={16} placeholder="16 символов" autoComplete="off"/>
      <button type="submit" disabled={busy||code.length!==16}>Подтвердить новое устройство</button>
     </form>
    </article>
   </div>
   <p role="status" aria-live="polite">{notice}</p>
   <p>Если на новом устройстве уже есть отдельный аккаунт, его автоматическая замена заблокирована ради сохранности данных. Это не объединение аккаунтов.</p>
  </section>
 </main>;
}
