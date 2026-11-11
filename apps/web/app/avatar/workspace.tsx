"use client";

import { useEffect, useRef, useState } from "react";

type Persona = { id: string; label: string; language: string; supports_video: boolean; supports_voice: boolean; supports_text: boolean };
type Catalog = { available: boolean; personas: Persona[] };
type Session = { session_id: string; persona_id: string; language: string; state: string; mode: string; safety_state: string };
type Transport = { launch_url: string; mode: string; expires_at: string };
type Entry = { side: "user" | "assistant"; text: string };
type Recognizer = { lang: string; onresult: ((event: { results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null; onerror: (() => void) | null; start: () => void; stop: () => void };
type SpeechWindow = Window & { SpeechRecognition?: new () => Recognizer; webkitSpeechRecognition?: new () => Recognizer };
const safetyNotice = "При непосредственной опасности обратитесь в местные экстренные службы или к человеку рядом. Экстренная помощь не требует покупки ИИ-сессии.";

export function AvatarWorkspace() {
 const [catalog,setCatalog] = useState<Catalog | null>(null);
 const [personaId,setPersonaId] = useState("");
 const [mode,setMode] = useState("TEXT");
 const [purpose,setPurpose] = useState("LISTEN");
 const [style,setStyle] = useState("CALM");
 const [busy,setBusy] = useState(false);
 const [session,setSession] = useState<Session | null>(null);
 const [transport,setTransport] = useState<Transport | null>(null);
 const [messages,setMessages] = useState<Entry[]>([]);
 const [draft,setDraft] = useState("");
 const [notice,setNotice] = useState("");
 const [voiceAvailable,setVoiceAvailable] = useState(false);
 const recognition = useRef<Recognizer | null>(null);
 const sending = useRef(false);
 useEffect(() => {
  let active=true;
  fetch("/v1/personas",{cache:"no-store",credentials:"same-origin"}).then(async response => {
   if(!response.ok) throw Error("Не удалось получить каталог");
   return response.json() as Promise<Catalog>;
  }).then(data => {
   if(!active) return;
   setCatalog(data);
   const first=data.personas[0];
   if(first){setPersonaId(first.id);setMode(first.supports_text ? "TEXT" : first.supports_voice ? "VOICE" : "VIDEO");}
  }).catch(()=>{if(active) setNotice("Каталог аватаров недоступен. Вы можете перейти к живому специалисту.");});
  // A page refresh loses local private messages, but the server's authenticated
  // lease can still be resumed without localStorage/sessionStorage transcripts.
  fetch("/v1/avatar-sessions/current",{credentials:"same-origin",cache:"no-store"})
   .then(async response=>{
    if(response.status===401)return null; // no existing cookie, not a failure
    if(!response.ok)throw Error("Статус предыдущего разговора пока не подтверждён.");
    return response.json();
   })
   .then(data=>{
    if(!active||!data)return;
    if(data.session?.state==="ACTIVE"){
     const resumed=data.session as Session;
     setSession(resumed);setPersonaId(resumed.persona_id);setMode(resumed.mode);
     setTransport({launch_url:"",mode:resumed.mode,expires_at:""});
     setMessages([]);setNotice("Соединение восстановлено. Переписка до перезагрузки не хранится на устройстве.");
    }else if(data.reason_code==="PERSONA_PROVIDER_SESSION_LOST"){
     setNotice("Провайдер перезапущен: старый разговор завершён. Можно начать новый.");
    }else if(data.reason_code==="PERSONA_PROVIDER_CLOSE_UNCERTAIN"){
     setNotice("Доступ к прежней сессии закрыт, но удаление у провайдера не подтверждено.");
    }
   })
   .catch(error=>{if(active)setNotice(error instanceof Error?error.message:"Не удалось проверить прошлый разговор.")});
  if(typeof window!=="undefined"){const w=window as SpeechWindow;setVoiceAvailable(Boolean(w.SpeechRecognition||w.webkitSpeechRecognition));}
  return ()=>{active=false;recognition.current?.stop();if(typeof window!=="undefined"&&"speechSynthesis" in window)window.speechSynthesis.cancel();};
 },[]);
 const selected=catalog?.personas.find(p=>p.id===personaId);
 const clearPrivate = () => {
  recognition.current?.stop();recognition.current=null;
  if("speechSynthesis" in window)window.speechSynthesis.cancel();
  setMessages([]);setDraft("");setTransport(null);
 };
 async function start() {
  if(!selected||busy)return;
  setBusy(true);setNotice("");
  try {
   const request={persona_id:personaId,session_purpose:purpose,mode,language:selected.language,style,privacy_mode:"PRIVATE_EPHEMERAL"};
   const response=await fetch("/v1/avatar-sessions",{method:"POST",headers:{"Content-Type":"application/json","Idempotency-Key":crypto.randomUUID()},body:JSON.stringify(request),credentials:"same-origin"});
   const data=await response.json();
   if(!response.ok)throw Error(data.message_safe||"PersonaProvider недоступен");
   if(data.session?.state!=="ACTIVE")throw Error("Провайдер не подтвердил активный разговор");
   setSession(data.session as Session);setTransport(data.transport as Transport);
   setMessages([]);
   setNotice(data.transport?.mode!==mode?"Выбран доступный режим вместо запрошенного.":"Сессия подключена. Можно разговаривать.");
  }catch(error){setNotice(error instanceof Error?error.message:"ИИ-персонаж недоступен");}
  finally{setBusy(false);}
 }
 async function send(raw: string) {
  const message=raw.trim();
  if(!session||session.state!=="ACTIVE"||!message||sending.current)return;
  if(message.length>2000){setNotice("Сообщение слишком длинное (не более 2000 символов).");return;}
  sending.current=true;setBusy(true);setNotice("");setDraft("");
  setMessages(prev=>[...prev,{side:"user",text:message}]);
  try {
   const response=await fetch(`/v1/avatar-sessions/${encodeURIComponent(session.session_id)}/turns`,{
    method:"POST",credentials:"same-origin",cache:"no-store",headers:{"Content-Type":"application/json"},body:JSON.stringify({message})
   });
   const data=await response.json();
   if(!response.ok||data.source!=="MODEL_GENERATED"||typeof data.reply!=="string"||!data.reply.trim()) {
    throw Error(data.message_safe||"ИИ-провайдер не ответил. Ответ не подменён заглушкой.");
   }
   setMessages(prev=>[...prev,{side:"assistant",text:data.reply}]);
   if(session.mode==="VOICE"&&"speechSynthesis" in window){
    window.speechSynthesis.cancel();
    const utterance=new SpeechSynthesisUtterance(data.reply);
    utterance.lang=session.language||"ru-RU";utterance.rate=0.95;
    window.speechSynthesis.speak(utterance);
   }
  }catch(error){setNotice(error instanceof Error?error.message:"Провайдер недоступен — ответа нет.");}
  finally{sending.current=false;setBusy(false);}
 }
 function speakInput() {
  if(!session||session.mode!=="VOICE"||busy)return;
  const w=window as SpeechWindow;
  const Ctor=w.SpeechRecognition||w.webkitSpeechRecognition;
  if(!Ctor){setNotice("Распознавание речи не поддерживается браузером. Можно написать текст.");return;}
  try{
   recognition.current?.stop();
   const rec=new Ctor();recognition.current=rec;rec.lang=session.language||"ru-RU";
   rec.onresult=(event)=>{const words=event.results[0]?.[0]?.transcript;if(words)void send(words);};
   rec.onerror=()=>setNotice("Микрофон или распознавание речи недоступны. Введите сообщение текстом.");
   rec.start();
  }catch{setNotice("Не удалось включить микрофон. Введите сообщение текстом.");}
 }
 async function act(action:"END"|"HUMAN_HANDOFF"|"SAFETY_ESCALATE") {
  if(!session||busy)return;
  setBusy(true);
  try{
   const response=await fetch(`/v1/avatar-sessions/${encodeURIComponent(session.session_id)}/actions`,{
    method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({action})
   });
   const data=await response.json();
   if(!response.ok)throw Error(data.message_safe||"Действие не выполнено");
   setSession(data.session as Session);
   if(action==="END"||action==="HUMAN_HANDOFF"||action==="SAFETY_ESCALATE")clearPrivate();
   if(data.provider_close==="DELIVERY_UNCERTAIN")setNotice("Локальная сессия заблокирована; внешнее удаление ещё не подтверждено.");
   else if(action==="HUMAN_HANDOFF")setNotice("Аватар отключён. Для разговора с человеком создайте отдельный запрос. Переписка не передаётся.");
   else if(action==="SAFETY_ESCALATE")setNotice(safetyNotice);
   else setNotice("Разговор завершён. Временный текст удалён из этого интерфейса.");
  }catch(error){setNotice(error instanceof Error?error.message:"Не удалось завершить действие");}
  finally{setBusy(false);}
 }
 return <section aria-label="AI-аватар" style={{display:"grid",gap:"1rem",marginTop:"2rem"}}>
  <p><strong>Это ИИ, не человек.</strong> Он не диагностирует и не лечит. Текст беседы хранится только во временной памяти страницы и провайдера, но не в профиле APGIC.</p>
  {notice&&<p role="status" aria-live="polite">{notice}</p>}
  {!session&&<>
   {!catalog?<p>Загружаем персонажей…</p>:!catalog.available?<p>Нет подключённого и проверенного провайдера. Можно воспользоваться обычным подбором специалиста.</p>:<>
    <label htmlFor="persona-choice">Персонаж</label>
    <select id="persona-choice" value={personaId} onChange={e=>{setPersonaId(e.target.value);const p=catalog.personas.find(x=>x.id===e.target.value);if(p)setMode(p.supports_text?"TEXT":p.supports_voice?"VOICE":"VIDEO");}}>
     {catalog.personas.map(p=><option key={p.id} value={p.id}>{p.label} — {p.language}</option>)}
    </select>
    <label htmlFor="persona-purpose">Цель общения</label>
    <select id="persona-purpose" value={purpose} onChange={e=>setPurpose(e.target.value)}>
     <option value="LISTEN">Выговориться</option><option value="ORGANIZE_THOUGHTS">Упорядочить мысли</option>
     <option value="PREPARE_HUMAN">Подготовиться к беседе с человеком</option><option value="WAIT_FOR_HUMAN">Скоротать ожидание</option><option value="NAVIGATE">Найти варианты помощи</option>
    </select>
    <label htmlFor="persona-mode">Режим</label>
    <select id="persona-mode" value={mode} onChange={e=>setMode(e.target.value)}>
     {selected?.supports_text&&<option value="TEXT">Текстовый разговор</option>}
     {selected?.supports_voice&&<option value="VOICE">Голосовой разговор (браузер)</option>}
     {selected?.supports_video&&<option value="VIDEO">Видеоперсонаж (внешний транспорт)</option>}
    </select>
    <label htmlFor="persona-style">Манера общения</label>
    <select id="persona-style" value={style} onChange={e=>setStyle(e.target.value)}>
     <option value="CALM">Спокойная</option><option value="NEUTRAL">Нейтральная</option><option value="WARM">Тёплая</option><option value="BUSINESS">Деловая</option>
    </select>
    <button type="button" className="button-link primary-action" onClick={start} disabled={busy||!selected}>Начать разговор</button>
   </>}
  </>}
  {session&&<div style={{display:"grid",gap:"1rem"}}>
   <p>Состояние: <strong>{session.state}</strong>. Режим: {session.mode}. ИИ-персонаж.</p>
   {session.mode==="VOICE"&&<p>При нажатии «Говорить в микрофон» запись может обрабатываться сервисом распознавания речи вашего браузера. APGIC не получает исходное аудио; для полной приватности используйте текстовый ввод.</p>}
   {session.state==="ACTIVE"&&transport?.launch_url&&<a href={transport.launch_url} target="_blank" rel="noopener noreferrer">Открыть внешний видеотранспорт</a>}
   {session.state==="ACTIVE"&&(!transport?.launch_url||session.mode!=="VIDEO")&&<>
    <div role="log" aria-live="polite" aria-label="Временная переписка" style={{display:"grid",gap:"0.6rem",padding:"0.75rem",border:"1px solid #bbb",borderRadius:8,maxHeight:360,overflowY:"auto"}}>
     {messages.length===0?<p>Напишите или произнесите первую фразу.</p>:messages.map((m,i)=><p key={i}><strong>{m.side==="user"?"Вы":"ИИ-персонаж"}:</strong> {m.text}</p>)}
    </div>
    <label htmlFor="avatar-message">Ваше сообщение</label>
    <textarea id="avatar-message" maxLength={2000} value={draft} onChange={e=>setDraft(e.target.value)} disabled={busy} rows={3}/>
    <button type="button" disabled={busy||!draft.trim()} onClick={()=>void send(draft)}>Отправить ИИ</button>
    {session.mode==="VOICE"&&<button type="button" disabled={busy||!voiceAvailable} onClick={speakInput}>Говорить в микрофон</button>}
    {session.mode==="VOICE"&&!voiceAvailable&&<p>Браузер не поддерживает распознавание речи; текстовый ввод остаётся доступен.</p>}
   </>}
   <button type="button" disabled={busy} onClick={()=>void act("HUMAN_HANDOFF")}>Перейти к человеку и отключить ИИ</button>
   <button type="button" disabled={busy} onClick={()=>void act("SAFETY_ESCALATE")}>Нужна срочная помощь — отключить ИИ</button>
   <button type="button" disabled={busy} onClick={()=>void act("END")}>Завершить и очистить разговор</button>
   <a href="/#start">Подобрать живого специалиста независимо от ИИ</a>
  </div>}
  <a href="/#start">Не использовать ИИ — перейти к специалистам</a>
  <p>{safetyNotice}</p>
 </section>;
}
