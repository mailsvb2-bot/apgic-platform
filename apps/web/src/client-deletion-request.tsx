"use client";

import {useCallback,useEffect,useRef,useState} from "react";

type Receipt={id:string;state:string;source:string;created_at:string;updated_at:string;notice:string;idempotent:boolean};
const endpoint="/v1/client/account-deletion-requests";
function receipt(value:unknown):Receipt{
  if(!value||typeof value!=="object")throw Error("Некорректный ответ сервера.");
  const x=value as Partial<Receipt>;
  if(!x.id||!x.state||!["WEB","IOS","ANDROID"].includes(x.source??"")||
     !x.created_at||!x.updated_at||typeof x.notice!=="string"||typeof x.idempotent!=="boolean")
    throw Error("Сервер не подтвердил регистрацию запроса.");
  return x as Receipt;
}
// The UI never claims erasure before the canonical server state proves it.
export default function DeletionRequestPanel({onSessionLost}:{onSessionLost:()=>void}){
  const [item,setItem]=useState<Receipt|null>(null);
  const [confirmed,setConfirmed]=useState(false);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [available,setAvailable]=useState(true);
  const epoch=useRef(0);
  const alive=useRef(true);
  const read=useCallback(async()=>{
    const token=++epoch.current;setError("");
    try{
      const response=await fetch(endpoint,{credentials:"include",cache:"no-store"});
      if(!alive.current||token!==epoch.current)return;
      if(response.status===401||response.status===403){onSessionLost();return;}
      if(response.status===404){setItem(null);return;}
      if(!response.ok)throw Error("Статус удаления временно недоступен.");
      const result=receipt(await response.json());
      if(alive.current&&token===epoch.current)setItem(result);
    }catch(e){if(alive.current&&token===epoch.current)setError(e instanceof Error?e.message:"Ошибка загрузки.");}
  },[onSessionLost]);
  useEffect(()=>{alive.current=true;void read();return()=>{alive.current=false;epoch.current++;};},[read]);
  const submit=async()=>{
    if(!confirmed||busy||item)return;
    const token=++epoch.current;setBusy(true);setError("");
    try{
      const response=await fetch(endpoint,{
        method:"POST",credentials:"include",cache:"no-store",
        headers:{"content-type":"application/json","accept":"application/json"},
        body:JSON.stringify({source:"WEB"}),
      });
      if(!alive.current||token!==epoch.current)return;
      if(response.status===401||response.status===403){onSessionLost();return;}
      if(!response.ok)throw Error("Не удалось зарегистрировать запрос. Повторите попытку.");
      const result=receipt(await response.json());
      if(alive.current&&token===epoch.current)setItem(result);
    }catch(e){if(alive.current&&token===epoch.current)setError(e instanceof Error?e.message:"Запрос не отправлен.");}
    finally{if(alive.current&&token===epoch.current)setBusy(false);}
  };
  return <section className="personal-section" aria-labelledby="delete-account-title">
    <h2 id="delete-account-title">Удаление учётной записи</h2>
    <p>Можно подать запрос на удаление. Регистрация запроса не означает, что данные уже стёрты. Часть сведений может сохраняться по закону.</p>
    {item?<div role="status">
      <p>Запрос № {item.id}</p><p>Статус сервера: <strong>{item.state}</strong></p>
      <p>{item.notice}</p>
      <button type="button" onClick={()=>void read()}>Обновить статус</button>
    </div>:<div>
      <label><input type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/>
        Я понимаю, что это запрос на удаление, а не подтверждение стирания данных.
      </label>
      <button type="button" disabled={!confirmed||busy||!available} onClick={()=>void submit()}>
        {busy?"Отправляем…":"Подать запрос на удаление аккаунта"}
      </button>
    </div>}
    {error?<p role="alert">{error}</p>:null}
  </section>;
}
