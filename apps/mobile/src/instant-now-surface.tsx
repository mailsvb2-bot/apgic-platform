import React, {useCallback,useEffect,useState} from "react";
import {Pressable,StyleSheet,Text,View} from "react-native";
import type {MobileDemandClientOptions} from "./mobile-demand-client.ts";
import {
  answerInstantDispatch,createInstantDispatch,listInstantInbox,listInstantSpecialistOffers,
  readActiveInstantDispatch,readInstantDispatch,readInstantPresence,updateInstantPresence,
  type InstantDispatch,type InstantPresence,type InstantSpecialistDispatch,type InstantSpecialistOffer,
} from "./instant-now-client.ts";

const styles=StyleSheet.create({
  card:{marginVertical:12,padding:16,borderRadius:12,borderWidth:1,borderColor:"#aeb7c4",gap:12},
  title:{fontWeight:"700",fontSize:19,color:"#202a3c"},
  body:{color:"#202a3c",fontSize:15},
  action:{padding:12,borderRadius:8,borderWidth:1,borderColor:"#858e9c",minHeight:44,justifyContent:"center"},
  actionText:{color:"#202a3c",fontSize:15,fontWeight:"600"},
  error:{color:"#a22020",fontWeight:"600"},
  detail:{fontSize:13,color:"#364451"},
  selected:{borderColor:"#2355a4",borderWidth:2},
});

export function InstantNowSurface({intentID,options,enabled,initialMode="client"}:{
 intentID?:string;options:MobileDemandClientOptions;enabled:boolean;
 initialMode?:"client"|"specialist";
}) {
 const [mode,setMode]=useState<"client"|"specialist">(initialMode);
 const [dispatch,setDispatch]=useState<InstantDispatch|null>(null);
 const [presence,setPresence]=useState<InstantPresence|null>(null);
 const [offers,setOffers]=useState<InstantSpecialistOffer[]>([]);
 const [selected,setSelected]=useState("");
 const [inbox,setInbox]=useState<InstantSpecialistDispatch[]>([]);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [message,setMessage]=useState("");
 const specialistRefresh=useCallback(async()=>{
  const [p,i]=await Promise.all([readInstantPresence(options),listInstantInbox(options)]);
  setPresence(p);setInbox(i);
 },[options.baseURL,options.sessionCookie]);
 useEffect(()=>{
  if(!enabled || mode!=="client")return;
  let active=true;
  void readActiveInstantDispatch(options).then(d=>{if(active)setDispatch(d);}).catch(()=>undefined);
  return ()=>{active=false;};
 },[enabled,mode,options.baseURL,options.sessionCookie]);
 useEffect(()=>{
  if(!enabled || mode!=="specialist")return;
  let active=true;
  void listInstantSpecialistOffers(options).then(os=>{
   if(active){setOffers(os);setSelected(os.find(o=>o.state==="ACTIVE")?.id||"");}
  }).catch(e=>{if(active)setError(e instanceof Error?e.message:"Не удалось загрузить предложения");});
  void specialistRefresh().catch(e=>{if(active)setError(e instanceof Error?e.message:"Ошибка дежурства");});
  return ()=>{active=false;};
 },[enabled,mode,options.baseURL,options.sessionCookie,specialistRefresh]);
 useEffect(()=>{
  if(!enabled)return;
  const interval=setInterval(()=>{
   if(mode==="specialist"){void specialistRefresh().catch(()=>setError("Нет связи с дежурством"));}
   else if(dispatch && ["OFFERED","ACCEPTED"].includes(dispatch.state)){
    void readInstantDispatch(dispatch.id,options).then(setDispatch).catch(()=>setError("Нет связи с заявкой"));
   }
  },3000);
  return ()=>clearInterval(interval);
 },[enabled,mode,dispatch?.id,dispatch?.state,options.baseURL,options.sessionCookie,specialistRefresh]);
 async function send(){
  if(!intentID)return;
  setBusy(true);setError("");setMessage("");
  try{setDispatch(await createInstantDispatch(intentID,options));}
  catch(e){setError(e instanceof Error?e.message:"Не удалось обратиться");}
  finally{setBusy(false);}
 }
 async function update(available:boolean){
  setBusy(true);setError("");setMessage("");
  try{
   setPresence(await updateInstantPresence(selected,available,options));
   setMessage(available?"Дежурство включено на десять минут.":"Дежурство выключено.");
  }catch(e){setError(e instanceof Error?e.message:"Не удалось обновить");}
  finally{setBusy(false);}
 }
 async function answer(id:string,accept:boolean){
  setBusy(true);setError("");setMessage("");
  try{
   const d=await answerInstantDispatch(id,accept,options);
   setMessage(d.state==="ACCEPTED"?"Предложение принято. Доступ к разговору возможен только после оплаты.":"Предложение отклонено.");
   await specialistRefresh();
  }catch(e){setError(e instanceof Error?e.message:"Не удалось ответить");}
  finally{setBusy(false);}
 }
 const choice=(label:string,onPress:()=>void,disabled=false,selectedMode=false)=><Pressable
   accessibilityRole="button" accessibilityLabel={label} disabled={busy||disabled}
   onPress={onPress} style={[styles.action,selectedMode?styles.selected:null]}>
    <Text style={styles.actionText}>{label}</Text>
   </Pressable>;
 return <View style={styles.card} accessibilityLabel="Дежурная консультация APGIC">
  <Text style={styles.title}>Консультация сейчас · Duty/Now</Text>
  <Text style={styles.body}>Выберите способ использования. Дежурство не означает подтверждённой оплаты или готовности комнаты.</Text>
  <View style={{flexDirection:"row",gap:8,flexWrap:"wrap"}}>
   {choice("Для клиента",()=>{setMode("client");setError("");},false,mode==="client")}
   {choice("Я специалист",()=>{setMode("specialist");setError("");},false,mode==="specialist")}
  </View>
  {mode==="client"? <>
    {!intentID&&<Text style={styles.body}>Сначала опишите запрос и подтвердите его темы выше.</Text>}
    {choice("Найти специалиста сейчас",()=>void send(),!enabled||!intentID||!!dispatch&&["OFFERED","ACCEPTED"].includes(dispatch.state))}
    {dispatch&&<>
     <Text accessibilityLabel={"native-duty-state:"+dispatch.state} style={styles.body}>
      {dispatch.state==="OFFERED"?"Ожидаем ответа специалиста":
       dispatch.state==="ACCEPTED"?"Специалист согласился. Требуется реальная внешняя оплата; комната пока закрыта.":
       dispatch.state==="UNFILLED"?"Нет подходящих дежурных. Можно записаться на другое время.":
       dispatch.state==="EXPIRED"?"Время предложения истекло.":
       "Предложение отклонено. Попробуйте снова или выберите запись."}
     </Text>
     {dispatch.price_minor? <Text style={styles.detail}>Стоимость предложения: {(dispatch.price_minor/100).toLocaleString("ru-RU")} {dispatch.currency}</Text>:null}
     <Text style={styles.detail}>Следующий шаг: {dispatch.next_action} · Доступ к комнате: закрыт</Text>
    </>}
  </>:<>
   <Text style={styles.body}>Доступность подтверждается только для действующего квалифицированного предложения.</Text>
   <Text style={styles.body}>Статус: {presence?.state||"Не загружен"}</Text>
   {offers.filter(o=>o.state==="ACTIVE").map(o=>choice(
    o.topic_id+" · "+(o.price_minor/100).toLocaleString("ru-RU")+" "+o.currency,
    ()=>setSelected(o.id),false,selected===o.id,
   ))}
   {offers.length===0&&<Text style={styles.body}>Создайте опубликованное ценовое предложение в кабинете специалиста.</Text>}
   {choice("Принимать сейчас",()=>void update(true),!enabled||!selected)}
   {choice("Завершить дежурство",()=>void update(false),!enabled)}
   <Text style={styles.title}>Предложения</Text>
   {inbox.length===0&&<Text style={styles.body}>Предложений пока нет.</Text>}
   {inbox.map(d=><View style={styles.card} key={d.id}>
    <Text style={styles.body}>{d.topic_id} · {d.state}</Text>
    {d.price_minor?<Text style={styles.detail}>{(d.price_minor/100).toLocaleString("ru-RU")} {d.currency}</Text>:null}
    {d.state==="OFFERED"&&<>
      {choice("Принять запрос",()=>void answer(d.id,true),!enabled)}
      {choice("Отклонить запрос",()=>void answer(d.id,false),!enabled)}
    </>}
   </View>)}
  </>}
  {message?<Text accessibilityRole="alert" style={styles.body}>{message}</Text>:null}
  {error?<Text accessibilityRole="alert" style={styles.error}>{error}</Text>:null}
  <Text style={styles.detail}>Экстренная помощь недоступна в APGIC. При непосредственной угрозе жизни обращайтесь в местные экстренные службы.</Text>
 </View>;
}
