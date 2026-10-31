"use client";

import {useEffect,useRef,useState} from "react";

type Receipt={id:string;state:string;source:string;created_at:string;updated_at:string;notice:string;idempotent:boolean;completed_at?:string;retained_reason?:string;pending_providers:number};
const endpoint="/v1/client/account-deletion-requests";

function receipt(value:unknown):Receipt{
  if(!value||typeof value!=="object")throw Error("Некорректный ответ сервера.");
  const x=value as Partial<Receipt>;
  if(!x.id||!x.state||!["WEB","IOS","ANDROID"].includes(x.source??"")||
    !x.created_at||!x.updated_at||typeof x.notice!=="string"||typeof x.idempotent!=="boolean")
    throw Error("Сервер не подтвердил регистрацию запроса.");
  return x as Receipt;
}
const deletionStates:Record<string,string>={
  REQUESTED:"Запрос получен. Подтвердите удаление данных.",
  IDENTITY_RECONFIRMED:"Личность подтверждена. Ожидается проверка правил хранения.",
  PROVIDER_ERASURE_PENDING:"Локальное удаление выполнено. Ожидается подтверждение внешних систем.",
  PARTIALLY_RETAINED_WITH_REASON:"Удаление завершено. Обязательные записи сохранены по правилам хранения.",
};
function deletionDate(value:string){
  const date=new Date(value);
  return Number.isNaN(date.getTime())?"Дата недоступна":date.toLocaleString("ru-RU");
}
// No local claims of data erasure or duplicate identity storage.
export default function DeletionRequestPanel({onSessionLost}:{onSessionLost:()=>void}){
  const [item,setItem]=useState<Receipt|null>(null);
  const [confirmed,setConfirmed]=useState(false);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const epoch=useRef(0);
  const alive=useRef(true);
  const busyRef=useRef(false);
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;};},[]);
  const execute=async(fn:(token:number)=>Promise<void>)=>{
    if(busyRef.current)return;
    busyRef.current=true;setBusy(true);setError("");
    const token=++epoch.current;
    try{await fn(token);}
    catch(e){if(alive.current&&token===epoch.current)
      setError(e instanceof Error?e.message:"Действие недоступно.");}
    finally{busyRef.current=false;if(alive.current&&token===epoch.current)setBusy(false);}
  };
  const accepted=(token:number)=>alive.current&&token===epoch.current;
  const read=()=>execute(async token=>{
    const res=await fetch(endpoint,{credentials:"include",cache:"no-store"});
    if(!accepted(token))return;
    if(res.status===401||res.status===403){onSessionLost();return;}
    if(res.status===404){setItem(null);return;}
    if(!res.ok)throw Error("Статус удаления временно недоступен.");
    const value=receipt(await res.json());
    if(accepted(token)){setItem(value);}
  });
  const submit=()=>execute(async token=>{
    if(!confirmed||item)return;
    const res=await fetch(endpoint,{method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","accept":"application/json"},
      body:JSON.stringify({source:"WEB"})});
    if(!accepted(token))return;
    if(res.status===401||res.status===403){onSessionLost();return;}
    if(!res.ok)throw Error("Запрос не зарегистрирован.");
    const value=receipt(await res.json());
    if(accepted(token))setItem(value);
  });
  const reconfirm=()=>execute(async token=>{
    if(!confirmed||item?.state!=="REQUESTED")return;
    const res=await fetch(endpoint+"/confirm",{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Confirmation":"explicit-session-v1"},
      body:JSON.stringify({confirm:true}),
    });
    if(!accepted(token))return;
    if(res.status===401||res.status===403){onSessionLost();return;}
    if(!res.ok)throw Error("Подтверждение удаления не принято сервером.");
    const value=receipt(await res.json());
    if(accepted(token)){setItem(value);setConfirmed(false);}
  });
  return <section className="personal-section" aria-labelledby="delete-account-title">
    <h2 id="delete-account-title">Удаление учётной записи</h2>
    <p>Удаление проходит через подтверждение личности и проверку правил хранения.
      Запрос не равен стиранию. Часть финансовых и юридических записей может сохраняться.</p>
    <button type="button" disabled={busy} onClick={()=>void read()}>Проверить статус запроса</button>
    {item?<div role="status">
      <p>Номер запроса: <code>{item.id}</code></p>
      <p>Состояние: <strong>{deletionStates[item.state]??"Статус уточняется на сервере"}</strong></p>
      <p>Получен: {deletionDate(item.created_at)}</p>
      <p>Последнее изменение: {deletionDate(item.updated_at)}</p>
      {item.completed_at?<p>Завершён: {deletionDate(item.completed_at)}</p>:null}
      {item.pending_providers>0?<p>Ожидается подтверждений внешних систем: {item.pending_providers}</p>:null}
      {item.retained_reason?<p>Причина сохранения обязательных данных: {item.retained_reason}</p>:null}
      <p>{item.notice}</p>
      <p>Ответ получен от сервера. Заявка сама по себе не доказывает удаления данных.</p>
      {item.state==="REQUESTED"?<div>
        <h3>Подтверждение удаления данных</h3>
        <p>Действие необратимо. Подтвердите удаление в текущей авторизованной сессии.
          Исполнение и проверка данных могут продолжаться после подтверждения.</p>
        <label><input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/>
          Я подтверждаю удаление моей учётной записи и персональных данных.
        </label>
        <button type="button" disabled={busy||!confirmed} onClick={()=>void reconfirm()}>
          Подтвердить удаление
        </button>
      </div>:null}
      <button type="button" disabled={busy} onClick={()=>void read()}>Обновить статус</button>
    </div>:<div>
      <label><input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/>
        Я понимаю, что это запрос на удаление, а не подтверждение стирания данных.
      </label>
      <button type="button" disabled={!confirmed||busy} onClick={()=>void submit()}>
        {busy?"Отправляем…":"Подать запрос на удаление аккаунта"}
      </button>
    </div>}
    {error?<p role="alert">{error}</p>:null}
  </section>;
}
