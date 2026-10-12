"use client";
import {useEffect,useRef,useState} from "react";

type Intent={id:string;topics:string[];goals:string[];context:Record<string,string>};
type Duty={id:string;help_intent_id?:string;state:string;topic_id?:string;price_minor?:number;currency?:string;expires_at?:string;next_action:string;room_ready:boolean};
type ApiError={code?:string;message_safe?:string};
const styles={panel:{background:"var(--surface, #fff)",border:"1px solid #ccc",borderRadius:14,padding:22,marginTop:16} as const,button:{padding:"0.7rem 1.1rem",borderRadius:8,marginTop:14,cursor:"pointer"} as const};
async function api<T>(path:string,body?:unknown,method="POST"):Promise<T>{
 const response=await fetch("/v1"+path,{method:body===undefined?"GET":method,credentials:"same-origin",cache:"no-store",headers:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body)});
 const result=await response.json() as T & ApiError;
 if(!response.ok)throw new Error(result.message_safe||result.code||"Не удалось выполнить запрос");
 return result;
}

export function NowJourney(){
 const [text,setText]=useState("");const [intent,setIntent]=useState<Intent|null>(null);
 const [topic,setTopic]=useState("");const [confirmed,setConfirmed]=useState(false);
 const [dispatch,setDispatch]=useState<Duty|null>(null);const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");const [notice,setNotice]=useState("");
 const retryKeyRef=useRef<string|null>(null);
 useEffect(()=>{
  // Reload/resume reads canonical server state, not a stale browser flag.
  void api<Duty>("/instant/active").then(setDispatch).catch(()=>undefined);
 },[]);
 useEffect(()=>{
  if(!dispatch || (dispatch.state!=="OFFERED" && dispatch.state!=="ACCEPTED"))return;
  const timer=window.setInterval(async()=>{
   try{const next=await api<Duty>("/instant/dispatches/"+encodeURIComponent(dispatch.id));setDispatch(next);}
   catch(e){setError(e instanceof Error?e.message:"Связь потеряна");}
  },3000);
  return ()=>window.clearInterval(timer);
 },[dispatch?.id,dispatch?.state]);
 async function create(){
  setBusy(true);setError("");setNotice("");setDispatch(null);setConfirmed(false);retryKeyRef.current=null;
  try{
   const next=await api<Intent>("/help-intents",{free_text:text});
   setIntent(next);setTopic(next.topics?.[0]||"");setNotice("Проверьте тему запроса. Платформа не ставит диагнозы.");
  }catch(e){setError(e instanceof Error?e.message:"Ошибка");}
  finally{setBusy(false);}
 }
 async function confirm(){
  if(!intent||!topic)return;setBusy(true);setError("");
  try{
   const next=await api<Intent>("/help-intents/"+encodeURIComponent(intent.id)+"/confirm",{topics:[topic],goals:intent.goals||[],context:intent.context||{}});
   setIntent(next);setConfirmed(true);setNotice("Запрос подтверждён. Можно обратиться к дежурному.");
  }catch(e){setError(e instanceof Error?e.message:"Ошибка");}
  finally{setBusy(false);}
 }
 async function request(){
  if(!intent)return;setBusy(true);setError("");
  try{
   if(dispatch && !["OFFERED","ACCEPTED"].includes(dispatch.state))retryKeyRef.current=null;
   if(!retryKeyRef.current){
    const {request_key}=await api<{request_key:string}>("/instant/request-key");
    if(!request_key)throw new Error("Не удалось создать безопасный ключ запроса.");
    retryKeyRef.current=request_key;
   }
   const next=await api<Duty>("/instant/dispatches",{help_intent_id:intent.id,request_key:retryKeyRef.current});
   if(next.room_ready)throw new Error("Недопустимый доступ к неоплаченной консультации.");
   setDispatch(next);
  }catch(e){
   // POST may have committed before the network response was lost.
   // Read the signed owner's durable state instead of double-dispatching.
   try{
    const recovered=await api<Duty>("/instant/active");
    if(recovered.help_intent_id===intent.id && !recovered.room_ready){
     setDispatch(recovered);return;
    }
   }catch{}
   setError(e instanceof Error?e.message:"Ошибка");
  }
  finally{setBusy(false);}
 }
 async function cancel(){
  if(!dispatch || busy)return;
  setBusy(true);setError("");
  try{
   const next=await api<Duty>("/instant/dispatches/"+encodeURIComponent(dispatch.id)+"/cancel",{},"POST");
   if(next.state!=="CANCELLED" || next.room_ready)throw new Error("Отмена не была подтверждена сервером.");
   setDispatch(next);retryKeyRef.current=null;setNotice("Обращение отменено. Деньги не списывались.");
  }catch(e){setError(e instanceof Error?e.message:"Не удалось отменить запрос.");}
  finally{setBusy(false);}
 }
 const terminal=dispatch && !["OFFERED","ACCEPTED"].includes(dispatch.state);
 return <section aria-label="Быстрая консультация" style={styles.panel}>
  {!intent&&!dispatch&&<>
   <label htmlFor="duty-description"><strong>С чем хотите обратиться?</strong></label>
   <textarea id="duty-description" value={text} onChange={e=>setText(e.target.value)} rows={4} maxLength={2000} placeholder="Опишите своими словами, что сейчас важно" style={{display:"block",width:"100%",marginTop:12}}/>
   <button type="button" style={styles.button} disabled={busy||!text.trim()} onClick={create}>Продолжить</button>
  </>}
  {intent&&!confirmed&&!dispatch&&<>
   <h2>Проверьте направление</h2>
   <p>Система предложила тему. Вы можете её исправить перед подтверждением.</p>
   <label htmlFor="duty-topic">Тема запроса</label>
   <select id="duty-topic" value={topic} onChange={e=>setTopic(e.target.value)} style={{display:"block",padding:10,width:"100%",marginTop:6}}>
    <option value="">Выберите тему</option>
    {[...new Set([...(intent.topics||[]),"anxiety","sleep","relationships","career"])].map(t=><option key={t} value={t}>{({anxiety:"Тревога и стресс",sleep:"Сон",relationships:"Отношения",career:"Карьера"} as Record<string,string>)[t]||t}</option>)}
   </select>
   <button type="button" style={styles.button} disabled={!topic||busy} onClick={confirm}>Подтвердить тему</button>
   <button type="button" style={{...styles.button,marginLeft:12}} onClick={()=>{setIntent(null);setTopic("");}}>Изменить описание</button>
  </>}
  {confirmed&&!dispatch&&<><h2>Запрос подтверждён</h2><p>Система направит предложение одному доступному квалифицированному специалисту. Время ответа ограничено.</p><button style={styles.button} type="button" disabled={busy} onClick={request}>Найти дежурного</button></>}
  {dispatch&&<div aria-live="polite">
   <h2>{dispatch.state==="OFFERED"?"Ожидаем ответ специалиста":dispatch.state==="ACCEPTED"?"Специалист ответил":dispatch.state==="UNFILLED"?"Свободного специалиста нет":dispatch.state==="DECLINED"?"Специалист не смог принять":dispatch.state==="EXPIRED"?"Предложение истекло":dispatch.state==="INVALIDATED"?"Предложение отозвано":"Статус предложения"}</h2>
   {dispatch.state==="OFFERED"&&<p>Предложение отправлено. Ожидание до {dispatch.expires_at?new Date(dispatch.expires_at).toLocaleTimeString("ru-RU"):"истечения предложения"}.</p>}
   {dispatch.state==="INVALIDATED"&&<p>Статус специалиста изменился. Данное предложение больше недействительно, к оплате или разговору по нему переходить нельзя.</p>}
   {dispatch.state==="ACCEPTED"&&<p>Специалист принял запрос. Для начала платной консультации требуется подтверждённый платёж через внешнего провайдера. Пока оплата не подключена, доступ к разговору закрыт.</p>}
   {dispatch.price_minor&&<p>Цена предложения: <strong>{(dispatch.price_minor/100).toLocaleString("ru-RU")} {dispatch.currency}</strong>. Списание сейчас не выполняется.</p>}
   {["OFFERED","ACCEPTED"].includes(dispatch.state)&&<button style={styles.button} type="button" disabled={busy} onClick={cancel}>Отменить обращение</button>}
   {terminal&&(intent ? <button style={styles.button} type="button" disabled={busy} onClick={request}>Попробовать другого</button> : <button type="button" style={styles.button} onClick={()=>{setDispatch(null);setConfirmed(false);}}>Начать новый запрос</button>)}
   <p style={{fontSize:13}}>Состояние: {dispatch.state} · Следующее действие: {dispatch.next_action}</p>
  </div>}
  {notice&&<p role="status">{notice}</p>}
  {error&&<p role="alert" style={{color:"#ba2525"}}>{error}</p>}
  <p style={{fontSize:13,marginTop:18}}>При экстренной угрозе жизни обратитесь в местную экстренную службу. APGIC не заменяет экстренную помощь.</p>
 </section>;
}
