import React,{useEffect,useRef,useState} from "react";
import {Pressable,Text,TextInput,View} from "react-native";

type Receipt={id:string;state:string;source:string;created_at:string;updated_at:string;notice:string;idempotent:boolean};
type Factor={enrolled:boolean;ready_at?:string;secret_base32?:string};
const path="/v1/client/account-deletion-requests";

function parse(v:unknown):Receipt{
  if(!v||typeof v!=="object")throw Error("invalid receipt");
  const x=v as Partial<Receipt>;
  if(!x.id||!x.state||!["WEB","IOS","ANDROID"].includes(x.source??"")||
     !x.created_at||!x.updated_at||typeof x.notice!=="string"||typeof x.idempotent!=="boolean")
    throw Error("invalid receipt");
  return x as Receipt;
}
function parseFactor(v:unknown):Factor{
  if(!v||typeof v!=="object"||typeof (v as Factor).enrolled!=="boolean")
    throw Error("invalid factor status");
  return v as Factor;
}

export default function ClientDeletionRequest({baseURL,source}:{baseURL:string;source:"IOS"|"ANDROID"}){
  const [state,setState]=useState<Receipt|null>(null);
  const [factor,setFactor]=useState<Factor|null>(null);
  const [secret,setSecret]=useState("");
  const [code,setCode]=useState("");
  const [verified,setVerified]=useState(false);
  const [confirmed,setConfirmed]=useState(false);
  const [busy,setBusy]=useState(false);
  const [message,setMessage]=useState("");
  const epoch=useRef(0);
  const alive=useRef(true);
  const busyRef=useRef(false);
  const address=(suffix="")=>new URL(path+suffix,baseURL).toString();
  const valid=(token:number)=>alive.current&&token===epoch.current;
  const act=async(fn:(token:number)=>Promise<void>)=>{
    if(busyRef.current)return;
    busyRef.current=true;setBusy(true);setMessage("");
    const token=++epoch.current;
    try{await fn(token);}
    catch{if(valid(token))setMessage("Действие не выполнено. Проверьте подключение и повторите попытку.");}
    finally{busyRef.current=false;if(valid(token))setBusy(false);}
  };
  const load=()=>act(async token=>{
    const res=await fetch(address(),{credentials:"include",cache:"no-store"});
    if(!valid(token))return;
    if(res.status===401||res.status===403){setState(null);setMessage("Требуется действующая сессия.");return;}
    if(res.status===404){setState(null);setFactor(null);setVerified(false);return;}
    if(!res.ok)throw Error("status unavailable");
    const result=parse(await res.json());
    if(valid(token)){setState(result);setVerified(false);}
  });
  // No background factor check: deletion operations cannot invalidate an
  // otherwise valid personal workspace before the user opens this action.
  useEffect(()=>{alive.current=true;void load();return()=>{alive.current=false;epoch.current++;};},[baseURL]);
  const submit=()=>act(async token=>{
    if(!confirmed||state)return;
    const res=await fetch(address(),{method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json"},body:JSON.stringify({source})});
    if(!valid(token))return;
    if(res.status===401||res.status===403){setState(null);setMessage("Требуется действующая сессия.");return;}
    if(!res.ok)throw Error("submit unavailable");
    const result=parse(await res.json());
    if(valid(token))setState(result);
  });
  const readFactor=()=>act(async token=>{
    const res=await fetch(address("/factor"),{credentials:"include",cache:"no-store"});
    if(!valid(token))return;
    if(res.status===401){setMessage("Требуется действующая сессия.");return;}
    if(!res.ok)throw Error("factor unavailable");
    const result=parseFactor(await res.json());
    if(valid(token))setFactor(result);
  });
  const enroll=()=>act(async token=>{
    const res=await fetch(address("/factor/enroll"),{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Factor":"enroll-v1"},
      body:JSON.stringify({acknowledge:true}),
    });
    if(!valid(token))return;
    if(!res.ok)throw Error("enrollment unavailable");
    const result=parseFactor(await res.json());
    if(!result.enrolled||!result.secret_base32)throw Error("missing enrollment secret");
    if(valid(token)){setFactor(result);setSecret(result.secret_base32);}
  });
  const verify=()=>act(async token=>{
    if(!/^\d{6}$/.test(code))throw Error("invalid code");
    const res=await fetch(address("/factor/verify"),{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Factor":"verify-v1"},
      body:JSON.stringify({code}),
    });
    if(!valid(token))return;
    if(!res.ok){setMessage("Код неверен или 24-часовой срок ожидания не закончился.");return;}
    if(valid(token)){setCode("");setSecret("");setVerified(true);}
  });
  const reconfirm=()=>act(async token=>{
    if(!verified)return;
    const res=await fetch(address("/reconfirm"),{
      method:"POST",credentials:"include",cache:"no-store",
      headers:{"content-type":"application/json","X-APGIC-Deletion-Confirmation":"verified-step-up-v1"},
      body:JSON.stringify({confirm:true}),
    });
    if(!valid(token))return;
    if(!res.ok){setMessage("Подтверждение отклонено. Проверьте одноразовый код.");return;}
    const result=parse(await res.json());
    if(valid(token)){setState(result);setVerified(false);}
  });
  const action=(label:string,callback:()=>void,disabled=false)=>
    <Pressable accessibilityRole="button" disabled={disabled||busy} onPress={callback}
      style={{padding:12,minHeight:48,borderWidth:1,borderRadius:8}}>
      <Text>{label}</Text>
    </Pressable>;
  return <View accessibilityLabel="Удаление учётной записи" style={{gap:8,paddingVertical:14}}>
    <Text style={{fontWeight:"700"}}>Удаление учётной записи</Text>
    <Text>Запрос не равен стиранию. До завершения сохраняются обязательные финансовые и юридические доказательства.</Text>
    {state?<View style={{gap:8}}>
      <Text>Запрос: {state.id}</Text><Text>Статус: {state.state}</Text><Text>{state.notice}</Text>
      {state.state==="REQUESTED"?<View style={{gap:8}}>
        <Text>Подтвердите личность через приложение одноразовых кодов.
          Новый фактор активируется через 24 часа.</Text>
        {action("Проверить готовность подтверждения",()=>void readFactor())}
        {!factor?.enrolled?action("Подключить приложение кодов",()=>void enroll()):null}
        {secret?<Text>Однократный секрет для приложения-аутентификатора: {secret}</Text>:null}
        {factor?.enrolled?<View style={{gap:8}}>
          <Text>Проверка доступна после: {factor.ready_at??"уточните статус"}</Text>
          <TextInput accessibilityLabel="Одноразовый код удаления" keyboardType="number-pad"
            maxLength={6} value={code} onChangeText={v=>setCode(v.replace(/\D/g,""))}
            style={{borderWidth:1,padding:12,borderRadius:8}}/>
          {action("Проверить код",()=>void verify(),code.length!==6)}
          {verified?action("Подтвердить личность для удаления",()=>void reconfirm()):null}
        </View>:null}
      </View>:null}
    </View>:<View style={{gap:8}}>
      <Pressable accessibilityRole="checkbox" accessibilityState={{checked:confirmed}}
        onPress={()=>setConfirmed(v=>!v)} style={{padding:12,borderWidth:1}}>
        <Text>{confirmed?"☑":"☐"} Понимаю, что подаю запрос, а не удаляю данные немедленно</Text>
      </Pressable>
      {action("Подать запрос на удаление",()=>void submit(),!confirmed)}
    </View>}
    {action("Обновить статус удаления",()=>void load())}
    {message?<Text accessibilityRole="alert">{message}</Text>:null}
  </View>;
}
