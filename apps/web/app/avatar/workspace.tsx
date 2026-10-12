"use client";

import { useEffect, useState } from "react";
type Persona = { id: string; label: string; language: string; supports_video: boolean; supports_voice: boolean; supports_text: boolean };
type Catalog = { available: boolean; personas: Persona[]; reason_code?: string };
type Session = { session_id: string; state: string; mode: string; safety_state: string };
type Started = { session: Session; transport: {launch_url: string; expires_at: string; mode: string} };
const safetyNotice = "При непосредственной опасности обратитесь в местные экстренные службы или к человеку рядом. Этот шаг доступен бесплатно.";
export function AvatarWorkspace(){
 const [catalog,setCatalog]=useState<Catalog|null>(null);
 const [personaId,setPersonaId]=useState("");
 const [mode,setMode]=useState("VOICE");
 const [purpose,setPurpose]=useState("LISTEN");
 const [style,setStyle]=useState("CALM");
 const [busy,setBusy]=useState(false);
 const [session,setSession]=useState<Session|null>(null);
 const [transport,setTransport]=useState<Started["transport"]|null>(null);
 const [notice,setNotice]=useState("");
 useEffect(()=>{
  let active=true;
  fetch("/v1/personas",{cache:"no-store"}).then(async r=>{if(!r.ok)throw Error("Каталог недоступен");return r.json() as Promise<Catalog>})
   .then(data=>{if(active){setCatalog(data);setPersonaId(data.personas[0]?.id||"");const p=data.personas[0];if(p)setMode(p.supports_voice?"VOICE":p.supports_text?"TEXT":"VIDEO");}})
   .catch(()=>{if(active)setNotice("Не удалось загрузить каталог аватаров. Вы можете выбрать живого специалиста.");});
  return ()=>{active=false};
 },[]);
 const selected=catalog?.personas.find(p=>p.id===personaId);
 async function start(){
  if(!selected)return;
  setBusy(true);setNotice("");
  try{
   const request={persona_id:personaId,session_purpose:purpose,mode,language:selected.language,style,privacy_mode:"PRIVATE_EPHEMERAL"};
   const response=await fetch("/v1/avatar-sessions",{method:"POST",headers:{"Content-Type":"application/json","Idempotency-Key":crypto.randomUUID()},body:JSON.stringify(request),credentials:"same-origin"});
   const result=await response.json();
   if(!response.ok)throw Error(result.message_safe||"Не удалось подключить аватар");
   const created=result as Started;setSession(created.session);setTransport(created.transport);
   setNotice(created.transport.mode!==mode?"Видеорежим недоступен: выбран безопасный запасной режим.":"Подключение готово. Откройте разговор в отдельной вкладке.");
  }catch(error){setNotice(error instanceof Error?error.message:"Аватар недоступен");}
  finally{setBusy(false)}
 }
 async function act(action:"END"|"HUMAN_HANDOFF"|"SAFETY_ESCALATE"){
  if(!session)return;
  setBusy(true);
  try{
   const response=await fetch(`/v1/avatar-sessions/${encodeURIComponent(session.session_id)}/actions`,{
    method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({action})});
   const result=await response.json();
   if(!response.ok)throw Error(result.message_safe||"Действие недоступно");
   setSession(result.session as Session);
   if(action==="HUMAN_HANDOFF")setNotice("Разговор не передаётся специалисту. Начните самостоятельный подбор ниже.");
   if(action==="SAFETY_ESCALATE")setNotice(safetyNotice);
   if(action==="END"){setTransport(null);setNotice(result.provider_close==="DELIVERY_UNCERTAIN"?
    "Сессия APGIC завершена, но подтверждения отключения внешнего провайдера нет. Закройте вкладку разговора.":"Разговор завершён. Содержание не сохранено в профиле APGIC.");}
  }catch(error){setNotice(error instanceof Error?error.message:"Действие недоступно")}
  finally{setBusy(false)}
 }
 return <section aria-label="AI-аватар" style={{display:"grid",gap:"1rem",marginTop:"2rem"}}>
  <p>По умолчанию действует <strong>приватный одноразовый режим</strong>: APGIC не сохраняет аудио, видео и расшифровку разговора и не передаёт их рекламе или специалистам.</p>
  {notice&&<p role="status" aria-live="polite">{notice}</p>}
  {!session&&<>
   {catalog===null?<p>Загружаем доступных персонажей…</p>:!catalog.available?<p>Сейчас безопасный разговор с аватаром не настроен. Запись к живому специалисту доступна отдельно.</p>:<>
    <label htmlFor="persona-choice">Аватар</label>
    <select id="persona-choice" value={personaId} onChange={e=>{setPersonaId(e.target.value);const p=catalog?.personas.find(x=>x.id===e.target.value);if(p)setMode(p.supports_voice?"VOICE":p.supports_text?"TEXT":"VIDEO");}}>
     {catalog.personas.map(p=><option key={p.id} value={p.id}>{p.label} — {p.language}</option>)}
    </select>
    <label htmlFor="persona-purpose">Цель разговора</label>
    <select id="persona-purpose" value={purpose} onChange={e=>setPurpose(e.target.value)}>
     <option value="LISTEN">Выговориться</option><option value="ORGANIZE_THOUGHTS">Упорядочить мысли</option>
     <option value="PREPARE_HUMAN">Подготовиться к беседе с человеком</option>
     <option value="WAIT_FOR_HUMAN">Скоротать ожидание</option><option value="NAVIGATE">Разобраться с вариантами помощи</option>
    </select>
    <label htmlFor="persona-mode">Режим</label>
    <select id="persona-mode" value={mode} onChange={e=>setMode(e.target.value)}>
     {selected?.supports_video&&<option value="VIDEO">Аватар и голос</option>}
     {selected?.supports_voice&&<option value="VOICE">Только голос</option>}
     {selected?.supports_text&&<option value="TEXT">Только текст</option>}
    </select>
    <label htmlFor="persona-style">Манера общения</label>
    <select id="persona-style" value={style} onChange={e=>setStyle(e.target.value)}>
     <option value="CALM">Спокойная</option><option value="NEUTRAL">Нейтральная</option>
     <option value="WARM">Тёплая</option><option value="BUSINESS">Деловая</option>
    </select>
    <button type="button" className="button-link primary-action" onClick={start} disabled={busy||!selected}>Начать приватный разговор</button>
   </>}
  </>}
  {session&&<div style={{display:"grid",gap:"1rem"}}>
   <p>Состояние: {session.state}. Режим: {session.mode}. <strong>AI-аватар</strong>.</p>
   {transport&&session.state==="ACTIVE"&&<a className="button-link primary-action" href={transport.launch_url}
    target="_blank" rel="noopener noreferrer">Открыть разговор с AI-аватаром</a>}
   <button type="button" disabled={busy} onClick={()=>act("HUMAN_HANDOFF")}>Хочу поговорить с человеком</button>
   <button type="button" disabled={busy} onClick={()=>act("SAFETY_ESCALATE")}>Нужна срочная помощь</button>
   {session.state==="ACTIVE"&&<button type="button" disabled={busy} onClick={()=>act("END")}>Завершить разговор</button>}
   <a href="/#start">Найти живого специалиста и выбрать время</a>
   <p>Передача разговора человеку не происходит автоматически. Подбор проверяет квалификацию заново.</p>
  </div>}
  <a href="/#start">Не использовать аватар — перейти к специалистам бесплатно</a>
  <p>{safetyNotice}</p>
 </section>;
}
