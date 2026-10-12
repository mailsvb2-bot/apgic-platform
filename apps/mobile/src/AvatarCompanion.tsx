import React,{useEffect,useState} from "react";
import {Linking,Pressable,Text,View} from "react-native";
import {actNativePersona,listNativePersonas,openNativePersona,
 type PersonaCard,type PersonaMode,type PersonaSession} from "./mobile-persona-client.ts";

export function AvatarCompanion({apiOrigin}:{apiOrigin:string}){
 const [catalog,setCatalog]=useState<PersonaCard[]>([]);
 const [available,setAvailable]=useState(false);
 const [selected,setSelected]=useState("");
 const [mode,setMode]=useState<PersonaMode>("TEXT");
 const [session,setSession]=useState<PersonaSession|null>(null);
 const [launchURL,setLaunchURL]=useState("");
 const [message,setMessage]=useState("");
 const [busy,setBusy]=useState(false);
 useEffect(()=>{
  let alive=true;
  listNativePersonas(apiOrigin).then(result=>{
   if(!alive)return;
   setCatalog(result.personas);setAvailable(result.available);
   if(result.personas.length){const first=result.personas[0];setSelected(first.id);setMode(first.supports_text?"TEXT":first.supports_voice?"VOICE":"VIDEO")}
  }).catch(()=>{if(alive)setMessage("Сервис аватаров пока недоступен.")});
  return()=>{alive=false};
 },[apiOrigin]);
 const choose=(p:PersonaCard)=>{setSelected(p.id);setMode(p.supports_text?"TEXT":p.supports_voice?"VOICE":"VIDEO")};
 async function start(){
  const p=catalog.find(x=>x.id===selected);if(!p)return;
  setBusy(true);
  try{
   const result=await openNativePersona(apiOrigin,{persona_id:p.id,session_purpose:"LISTEN",mode,language:p.language,
    style:"CALM",privacy_mode:"PRIVATE_EPHEMERAL"},`avatar-${Date.now()}-${Math.random().toString(36).slice(2)}`);
   setSession(result.session);setLaunchURL(result.transport.launch_url);
   setMessage(result.transport.mode===mode?"Разговор готов.":"Запрошенный режим недоступен: выбран запасной режим.");
  }catch(e){setMessage(e instanceof Error?e.message:"Аватар недоступен")}
  finally{setBusy(false)}
 }
 async function action(kind:"END"|"HUMAN_HANDOFF"|"SAFETY_ESCALATE"){
  if(!session)return;
  setBusy(true);
  try{
   const updated=await actNativePersona(apiOrigin,session.session_id,kind);
   setSession(updated.session);
   if(kind==="END"){setLaunchURL("");setMessage(updated.provider_close==="DELIVERY_UNCERTAIN"?
    "APGIC завершил сессию, но внешний провайдер не подтвердил отключение. Закройте окно разговора.":"Разговор завершён. APGIC не сохраняет его содержание.");}
   if(kind==="HUMAN_HANDOFF")setMessage("Разговор не передан человеку. Для записи создайте отдельный запрос на подбор.");
   if(kind==="SAFETY_ESCALATE")setMessage("При непосредственной опасности обратитесь в местные экстренные службы или к человеку рядом. Оплата не требуется.");
  }catch(e){setMessage(e instanceof Error?e.message:"Действие недоступно")}
  finally{setBusy(false)}
 }
 const button=(label:string,onPress:()=>void)=><Pressable accessibilityRole="button" accessibilityLabel={label}
  style={{borderWidth:1,borderRadius:8,padding:12,minHeight:48}} disabled={busy} onPress={onPress}><Text>{label}</Text></Pressable>;
 const item=catalog.find(x=>x.id===selected);
 return <View style={{gap:10,padding:12,borderWidth:1,borderRadius:12}}>
  <Text accessibilityRole="header" style={{fontSize:22,fontWeight:"700"}}>Поговорить с AI-аватаром</Text>
  <Text>AI-аватар не является человеком, врачом или психологом, не диагностирует и не проводит терапию.</Text>
  <Text>Приватный одноразовый режим. Разговор не записывается в профиль и не передаётся рекламе.</Text>
  {message?<Text accessibilityRole="alert">{message}</Text>:null}
  {!session&&available?<>
   {catalog.map(p=><React.Fragment key={p.id}>{button((p.id===selected?"✓ ":"")+p.label,()=>choose(p))}</React.Fragment>)}
   {item?.supports_video?button((mode==="VIDEO"?"✓ ":"")+"Аватар и голос",()=>setMode("VIDEO")):null}
   {item?.supports_voice?button((mode==="VOICE"?"✓ ":"")+"Только голос",()=>setMode("VOICE")):null}
   {item?.supports_text?button((mode==="TEXT"?"✓ ":"")+"Только текст",()=>setMode("TEXT")):null}
   {button("Начать приватный разговор",()=>void start())}
  </>:!session?<Text>Сейчас AI-аватар недоступен. Обычный подбор специалиста остаётся отдельным.</Text>:<>
   <Text>Состояние: {session.state}. Режим: {session.mode}</Text>
   {session.state==="ACTIVE"&&launchURL?button("Открыть разговор",()=>{void Linking.openURL(launchURL)}):null}
   {button("Хочу поговорить с человеком",()=>void action("HUMAN_HANDOFF"))}
   {button("Нужна срочная помощь",()=>void action("SAFETY_ESCALATE"))}
   {session.state==="ACTIVE"?button("Завершить разговор",()=>void action("END")):null}
  </>}
  <Text>Помощь человека и экстренная навигация не требуют покупки разговора с аватаром.</Text>
 </View>;
}
