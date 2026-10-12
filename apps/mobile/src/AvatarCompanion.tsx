import React,{useEffect,useState} from "react";
import {Linking,Pressable,ScrollView,Text,TextInput,View} from "react-native";
import {actNativePersona,listNativePersonas,openNativePersona,sendNativePersona,
 type PersonaCard,type PersonaMode,type PersonaSession} from "./mobile-persona-client.ts";

type Entry={side:"user"|"assistant";text:string};
export function AvatarCompanion({apiOrigin}:{apiOrigin:string}){
 const [catalog,setCatalog]=useState<PersonaCard[]>([]);
 const [available,setAvailable]=useState(false);
 const [selected,setSelected]=useState("");
 const [mode,setMode]=useState<PersonaMode>("TEXT");
 const [session,setSession]=useState<PersonaSession|null>(null);
 const [launchURL,setLaunchURL]=useState("");
 const [message,setMessage]=useState("");
 const [draft,setDraft]=useState("");
 const [lines,setLines]=useState<Entry[]>([]);
 const [busy,setBusy]=useState(false);
 useEffect(()=>{
  let alive=true;
  listNativePersonas(apiOrigin).then(result=>{
   if(!alive)return;
   setCatalog(result.personas);setAvailable(result.available);
   if(result.personas.length){const p=result.personas[0];setSelected(p.id);setMode(p.supports_text?"TEXT":p.supports_video?"VIDEO":"VOICE");}
  }).catch(()=>{if(alive)setMessage("Подключённого безопасного PersonaProvider пока нет.")});
  return()=>{alive=false};
 },[apiOrigin]);
 const choose=(p:PersonaCard)=>{setSelected(p.id);setMode(p.supports_text?"TEXT":p.supports_video?"VIDEO":"VOICE")};
 async function start(){
  const p=catalog.find(x=>x.id===selected);if(!p||busy)return;
  setBusy(true);setMessage("");
  try{
   const result=await openNativePersona(apiOrigin,{persona_id:p.id,session_purpose:"LISTEN",mode,language:p.language,
    style:"CALM",privacy_mode:"PRIVATE_EPHEMERAL"},`avatar-${Date.now()}-${Math.random().toString(36).slice(2)}`);
   setSession(result.session);setLaunchURL(result.transport.launch_url);setLines([]);
   setMessage(result.transport.mode===mode?"Сессия готова для разговора.":"Провайдер согласовал другой режим разговора.");
  }catch(e){setMessage(e instanceof Error?e.message:"ИИ-персонаж недоступен")}
  finally{setBusy(false)}
 }
 async function send(){
  if(!session||session.state!=="ACTIVE"||busy||!draft.trim())return;
  const input=draft.trim();setDraft("");setLines(prev=>[...prev,{side:"user",text:input}]);setBusy(true);setMessage("");
  try{
   const answer=await sendNativePersona(apiOrigin,session.session_id,input);
   setLines(prev=>[...prev,{side:"assistant",text:answer}]);
  }catch(e){setMessage(e instanceof Error?e.message:"Провайдер не ответил. Поддельный ответ не показан.")}
  finally{setBusy(false)}
 }
 async function action(kind:"END"|"HUMAN_HANDOFF"|"SAFETY_ESCALATE"){
  if(!session||busy)return;
  setBusy(true);
  try{
   const updated=await actNativePersona(apiOrigin,session.session_id,kind);
   setSession(updated.session);setLaunchURL("");setLines([]);setDraft("");
   if(updated.provider_close==="DELIVERY_UNCERTAIN")setMessage("Доступ закрыт, но удаление у внешнего провайдера пока не подтверждено.");
   else if(kind==="HUMAN_HANDOFF")setMessage("Аватар остановлен. Сформулируйте отдельный запрос живому специалисту; история не передаётся.");
   else if(kind==="SAFETY_ESCALATE")setMessage("При непосредственной опасности обратитесь в местную экстренную службу или к человеку рядом.");
   else setMessage("Разговор завершён. Временные сообщения удалены.");
  }catch(e){setMessage(e instanceof Error?e.message:"Невозможно подтвердить завершение")}
  finally{setBusy(false)}
 }
 const button=(label:string,onPress:()=>void,disabled=false)=><Pressable accessibilityRole="button" accessibilityLabel={label}
  style={{borderWidth:1,borderRadius:8,padding:12,minHeight:48,opacity:disabled?0.6:1}} disabled={busy||disabled} onPress={onPress}><Text>{label}</Text></Pressable>;
 const item=catalog.find(x=>x.id===selected);
 return <View style={{gap:10,padding:12,borderWidth:1,borderRadius:12}}>
  <Text accessibilityRole="header" style={{fontSize:22,fontWeight:"700"}}>Поговорить с ИИ-персонажем</Text>
  <Text>ИИ — не человек и не специалист. Не диагностирует, не лечит, не заменяет экстренную помощь.</Text>
  <Text>Сообщения хранятся временно в памяти приложения и в памяти провайдера, но не сохраняются в APGIC.</Text>
  {message?<Text accessibilityRole="alert">{message}</Text>:null}
  {!session&&available?<>
   {catalog.map(p=><React.Fragment key={p.id}>{button((p.id===selected?"✓ ":"")+p.label,()=>choose(p))}</React.Fragment>)}
   {item?.supports_text?button((mode==="TEXT"?"✓ ":"")+"Текстовый разговор",()=>setMode("TEXT")):null}
   {item?.supports_video?button((mode==="VIDEO"?"✓ ":"")+"Видео через внешний транспорт",()=>setMode("VIDEO")):null}
   {item?.supports_voice?button((mode==="VOICE"?"✓ ":"")+"Голос через внешний транспорт",()=>setMode("VOICE")):null}
   {button("Начать приватный разговор",()=>void start())}
  </>:!session?<Text>Нет подключённого проверенного ИИ-провайдера. Живой специалист доступен независимо от ИИ.</Text>:<>
   <Text>Состояние: {session.state}. Режим: {session.mode}</Text>
   {session.state==="ACTIVE"&&launchURL?button("Открыть внешний разговор",()=>{void Linking.openURL(launchURL)}):null}
   {session.state==="ACTIVE"&&!launchURL&&session.mode==="VOICE"?<Text>На мобильном устройстве голосовой ввод без внешнего транспорта пока не поддерживается; доступен текстовый ввод.</Text>:null}
   {session.state==="ACTIVE"&&!launchURL?<View style={{gap:8}}>
    <ScrollView style={{maxHeight:260}} accessibilityLabel="Временные сообщения">
     {lines.map((x,i)=><Text key={i} style={{padding:6}}><Text style={{fontWeight:"bold"}}>{x.side==="user"?"Вы: ":"ИИ: "}</Text>{x.text}</Text>)}
    </ScrollView>
    <TextInput accessibilityLabel="Сообщение ИИ-персонажу" multiline maxLength={2000} value={draft} onChangeText={setDraft}
     editable={!busy} placeholder="Введите сообщение" style={{borderWidth:1,borderRadius:8,padding:10,minHeight:58}}/>
    {button("Отправить ИИ",()=>void send(),!draft.trim())}
   </View>:null}
   {button("Перейти к человеку — отключить ИИ",()=>void action("HUMAN_HANDOFF"))}
   {button("Нужна срочная помощь — отключить ИИ",()=>void action("SAFETY_ESCALATE"))}
   {button("Завершить и очистить разговор",()=>void action("END"))}
  </>}
  <Text>Переход к специалисту требует нового запроса, без передачи приватного разговора.</Text>
 </View>;
}
