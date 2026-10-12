import React, {useEffect, useState} from "react";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";
import {Linking, Pressable, StyleSheet, Text, View} from "react-native";
import {
 fetchNativePublicSpecialist,
 fetchNativePublicOffers,
 fetchNativeSpecialistDirectory,
 specialistBookingHandoffURL,
 type PublicSpecialist,
} from "./mobile-specialist-directory.ts";
import type {PublicBookableOfferList} from "../../../packages/contracts/src/generated/apgic-v1.ts";

const topicNames: Record<string,string> = {
 anxiety:"Тревога и стресс", sleep:"Сон", relationships:"Отношения", career:"Карьера и работа",
};

export function NativeSpecialistDirectory({baseURL,topic}:{baseURL:string;topic:string}) {
 const [profiles,setProfiles]=useState<PublicSpecialist[]>([]);
 const [selected,setSelected]=useState<PublicSpecialist|null>(null);
 const [offers,setOffers]=useState<PublicBookableOfferList|null>(null);
 const [offersUnavailable,setOffersUnavailable]=useState(false);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState("");
 const [notice,setNotice]=useState("");
 useEffect(()=>{
  let active=true;
  const controller=new AbortController();
  setProfiles([]);setSelected(null);setOffers(null);setLoading(true);setError("");setNotice("");
  void fetchNativeSpecialistDirectory(baseURL,topic,controller.signal)
   .then(payload=>{if(active){setProfiles(payload.profiles);setLoading(false);}})
   .catch(()=>{if(active){setError("Каталог временно недоступен или тема пока не поддерживается.");setLoading(false);}});
  return ()=>{active=false;controller.abort();};
 },[baseURL,topic]);
 async function select(p:PublicSpecialist) {
  // Never trust a cached search card to remain published after review/revocation.
  setError("");setNotice("");setSelected(null);setOffers(null);setOffersUnavailable(false);
  try {
   const fresh=await fetchNativePublicSpecialist(baseURL,p.id);
   if(!fresh){setNotice("Анкета больше не опубликована. Обновите каталог.");return;}
   setSelected(fresh);
   try{
    const current=await fetchNativePublicOffers(baseURL,fresh.id);
    if(!current){setSelected(null);setNotice("Публикация отозвана. Обновите каталог.");return;}
    setOffers(current);
   }catch{setOffersUnavailable(true)}

  }catch{setError("Не удалось проверить актуальность анкеты.");}
 }
 return <View style={styles.directory} accessibilityLabel="Проверенный каталог специалистов">
  <Text style={styles.title}>Проверенные специалисты</Text>
  <Text style={styles.note}>Направление: {topicNames[topic]??topic}. Отображаются только анкеты с действующей публикацией и допуском.</Text>
  {loading?<Text accessibilityRole="alert">Загружаем специалистов…</Text>:null}
  {error?<Text accessibilityRole="alert">{error}</Text>:null}
  {!loading&&!error&&profiles.length===0?<Text>Опубликованных анкет по выбранной теме сейчас нет.</Text>:null}
  {profiles.map(profile=><Pressable key={profile.id} accessibilityRole="button"
    accessibilityLabel={`Открыть анкету ${profile.display_name}`}
    style={styles.card} onPress={()=>void select(profile)}>
   <Text style={styles.name}>{profile.display_name}</Text>
   <Text>{profile.profession_code==="PSYCHOLOGIST"?"Психолог":profile.profession_code==="CAREER_COACH"?"Карьерный консультант":"Специалист"}</Text>
   {profile.headline?<Text>{profile.headline}</Text>:null}
   <Text>{profile.published_topics.map(topic=>topicNames[topic]??topic).join(", ")}</Text>
   <Text style={styles.link}>Посмотреть подтверждённую анкету</Text>
  </Pressable>)}
  {notice?<Text accessibilityRole="alert">{notice}</Text>:null}
  {selected?<View style={styles.detail} accessibilityLabel="Выбранная анкета специалиста">
   <Text style={styles.name}>{selected.display_name}</Text>
   {selected.about?<Text>{selected.about}</Text>:null}
   {selected.experience_years>0?<Text>Заявленный опыт: {selected.experience_years} лет</Text>:null}
   {selected.verified_review_count>=5&&typeof selected.verified_rating==="number"?
    <Text>Подтверждённый рейтинг: {selected.verified_rating.toFixed(1)} / 5, оценок: {selected.verified_review_count}</Text>:
    <Text>Проверенный рейтинг пока не сформирован</Text>}
   {selected.limitations.length>0?<Text>Ограничения: {selected.limitations.join("; ")}</Text>:null}
   {offersUnavailable?<Text accessibilityRole="alert">Стоимость и доступность сейчас не подтверждены. Проверьте их при записи.</Text>:null}
   {offers && offers.offers.length===0?<Text>Сейчас нет доступных платных слотов.</Text>:null}
   {offers?.offers.map(offer=><View key={offer.id} style={styles.card}>
    <Text style={styles.name}>{topicNames[offer.topic_id]??offer.topic_id}</Text>
    <Text>{new Intl.NumberFormat("ru-RU").format(offer.price_minor/100)} ₽ / консультация</Text>
    {offer.slots.map(slot=><Text key={slot.slot_id}>
     {new Date(slot.starts_at).toLocaleString("ru-RU")} — {Math.round((new Date(slot.ends_at).getTime()-new Date(slot.starts_at).getTime())/60000)} минут
    </Text>)}
    <Pressable accessibilityRole="button" accessibilityLabel={`Выбрать услугу ${topicNames[offer.topic_id]??offer.topic_id}`}
      style={styles.action} onPress={()=>void Linking.openURL(specialistBookingHandoffURL(canonicalAPGICOrigin,selected.id,offer.id))}>
     <Text style={styles.actionText}>Выбрать эту услугу и перейти к записи</Text>
    </Pressable>
   </View>)}
   <Text style={styles.note}>Цены и свободное время берутся из действующих предложений. Просмотр анкеты не резервирует слот и не создаёт оплату.</Text>
   {!offers || offers.offers.length===0?<Pressable accessibilityRole="button" accessibilityLabel="Описать запрос к выбранному специалисту"
     style={styles.action} onPress={()=>void Linking.openURL(specialistBookingHandoffURL(canonicalAPGICOrigin,selected.id))}>
    <Text style={styles.actionText}>Уточнить запрос и проверить запись</Text>
   </Pressable>:null}
  </View>:null}
 </View>;
}
const styles=StyleSheet.create({
 directory:{padding:12,gap:12,borderWidth:1,borderColor:"#a4c2af",borderRadius:16,marginTop:15,backgroundColor:"#f1f8f2"},
 title:{fontSize:18,fontWeight:"700",color:"#173d2e"},
 note:{fontSize:13,color:"#365c46"},
 card:{borderWidth:1,borderColor:"#c5daca",padding:12,borderRadius:12,backgroundColor:"white",gap:6},
 name:{fontSize:16,fontWeight:"700",color:"#163c2b"},
 link:{fontSize:13,color:"#245b39",fontWeight:"600"},
 detail:{gap:10,borderTopWidth:1,borderTopColor:"#a4c2af",paddingTop:12},
 action:{borderRadius:12,backgroundColor:"#163f31",padding:13,alignItems:"center"},
 actionText:{fontWeight:"700",color:"white"},
});
