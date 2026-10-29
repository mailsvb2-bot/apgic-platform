"use client";

import {useEffect,useRef,useState} from "react";

type Receipt={id:string;state:string;source:string;created_at:string;updated_at:string;notice:string;idempotent:boolean};
type FactorStatus={enrolled:boolean;ready_at?:string;secret_base32?:string};
const endpoint="/v1/client/account-deletion-requests";

function receipt(value:unknown):Receipt{
  if(!value||typeof value!=="object")throw Error("Некорректный ответ сервера.");
  const x=value as Partial<Receipt>;
  if(!x.id||!x.state||!["WEB","IOS","ANDROID"].includes(x.source??"")||
    !x.created_at||!x.updated_at||typeof x.notice!=="string"||typeof x.idempotent!=="boolean")
    throw Error("Сервер не подтвердил регистрацию запроса.");
  return x as Receipt;
}
function factorStatus(value:unknown):FactorStatus{
  if(!value||typeof value!=="object"||typeof (value as FactorStatus).enrolled!=="boolean")
    throw Error("Сервер не подтвердил статус проверки личности.");
  return value as FactorStatus;
}

// No local claims of data erasure or duplicate identity storage.
export default function DeletionRequestPanel({onSessionLost}:{onSessionLost:()=>void}){
  const [item,setItem]=useState<Receipt|null>(null);
  const [factor,setFactor]=useState<FactorStatus|null>(null);
  const [secret,setSecret]=useState("");
  const [otp,setOtp]=useState("");
  const [verified,setVerified]=useState(false);
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
    if(res.status===404){setItem(null);setFactor(null);setVerified(false);return;}
    if(!res.ok)throw Error("Статус удаления временно недоступен.");
    const value=receipt(await res.json());
    if(accepted(token)){setItem(value);setVerified(false);}
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
  const readFactor=()=>execute(async token=>{
    const res=await fetch(endpoint+"/factor",{credentials:"include",cache:"no-store"});
    if(!accepted(token))return;
    if(res.status===401){onSessionLost();return;}
    if(!res.ok)throw Error("Подтверждение личности пока недоступно.");
    const value=factorStatus(await res.json());
    if(accepted(token))setFactor(value);
  });
  const enroll=()=>execute(async token=>{
    const res=await fetch(endpoint+"/factor/enroll",{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Factor":"enroll-v1"},
      body:JSON.stringify({acknowledge:true}),
    });
    if(!accepted(token))return;
    if(res.status===401){onSessionLost();return;}
    if(!res.ok)throw Error("Фактор уже подключён или регистрация недоступна.");
    const value=factorStatus(await res.json());
    if(!value.enrolled||!value.secret_base32)throw Error("Секрет настройки не получен.");
    if(accepted(token)){setFactor(value);setSecret(value.secret_base32);}
  });
  const verify=()=>execute(async token=>{
    if(!/^\d{6}$/.test(otp))throw Error("Введите шестизначный код.");
    const res=await fetch(endpoint+"/factor/verify",{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Factor":"verify-v1"},
      body:JSON.stringify({code:otp}),
    });
    if(!accepted(token))return;
    if(res.status===401){onSessionLost();return;}
    if(!res.ok)throw Error("Код не принят или 24-часовой срок ожидания не завершён.");
    if(accepted(token)){setVerified(true);setOtp("");setSecret("");}
  });
  const reconfirm=()=>execute(async token=>{
    if(!verified)return;
    const res=await fetch(endpoint+"/reconfirm",{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Confirmation":"verified-step-up-v1"},
      body:JSON.stringify({confirm:true}),
    });
    if(!accepted(token))return;
    if(res.status===401){onSessionLost();return;}
    if(!res.ok)throw Error("Повторное подтверждение отклонено. Проверьте действительность кода.");
    const value=receipt(await res.json());
    if(accepted(token)){setItem(value);setVerified(false);}
  });
  return <section className="personal-section" aria-labelledby="delete-account-title">
    <h2 id="delete-account-title">Удаление учётной записи</h2>
    <p>Удаление проходит через подтверждение личности и проверку правил хранения.
      Запрос не равен стиранию. Часть финансовых и юридических записей может сохраняться.</p>
    <button type="button" disabled={busy} onClick={()=>void read()}>Проверить статус запроса</button>
    {item?<div role="status">
      <p>Запрос № {item.id}</p><p>Статус сервера: <strong>{item.state}</strong></p>
      <p>{item.notice}</p>
      {item.state==="REQUESTED"?<div>
        <h3>Повторное подтверждение личности</h3>
        <p>Для защиты аккаунта подключите приложение одноразовых кодов.
          После подключения требуется 24 часа ожидания.</p>
        <button type="button" disabled={busy} onClick={()=>void readFactor()}>Проверить готовность подтверждения</button>
        {!factor?.enrolled?<button type="button" disabled={busy} onClick={()=>void enroll()}>
          Подключить приложение кодов
        </button>:null}
        {secret?<p>Сохраните секрет в приложении-аутентификаторе. Он показывается только один раз:
          <code>{secret}</code></p>:null}
        {factor?.enrolled?<div>
          <p>Подтверждение доступно после: {factor.ready_at??"уточните статус"}</p>
          <label>Код из приложения
            <input inputMode="numeric" autoComplete="one-time-code" maxLength={6}
              value={otp} onChange={e=>setOtp(e.target.value.replace(/\D/g,""))}/>
          </label>
          <button type="button" disabled={busy||otp.length!==6} onClick={()=>void verify()}>
            Проверить код
          </button>
          {verified?<button type="button" disabled={busy} onClick={()=>void reconfirm()}>
            Подтвердить личность для удаления
          </button>:null}
        </div>:null}
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
