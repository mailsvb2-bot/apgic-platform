import React,{useCallback,useState} from "react";
import {Pressable,Text,View} from "react-native";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";

export type NativeAcademyCourse={
 id:string;title:string;description:string;version_no:number;version_id:string;
};
export type NativeAcademyEnrollment={
 id:string;version_id:string;state:"ACTIVE"|"COMPLETED"|"REVOKED";
};
export type NativeAcademyLesson={
 id:string;title:string;kind:"READING"|"VIDEO"|"ASSESSMENT";body?:string;completed:boolean;
};
export type NativeAcademyCertificate={
 verification_url:string;state:"ACTIVE"|"REVOKED";evidence_candidate_state:string;
};
type AcademyFetch=(url:string,init?:RequestInit)=>Promise<Response>;

export async function academyRequest<T>(
 path:string,method:"GET"|"POST"="GET",fetchImpl:AcademyFetch=fetch,origin=canonicalAPGICOrigin,
):Promise<T>{
 const response=await fetchImpl(new URL(path,origin).toString(),{
  method,credentials:"include",headers:{"Accept":"application/json"},
 });
 const value:unknown=await response.json();
 if(!response.ok){
  const err=value as {message_safe?:string;code?:string};
  throw new Error(err.message_safe||err.code||"ACADEMY_UNAVAILABLE");
 }
 return value as T;
}

export function AcademySurface(){
 const [courses,setCourses]=useState<NativeAcademyCourse[]>([]);
 const [enrollments,setEnrollments]=useState<NativeAcademyEnrollment[]>([]);
 const [lessons,setLessons]=useState<NativeAcademyLesson[]>([]);
 const [selected,setSelected]=useState<NativeAcademyEnrollment|null>(null);
 const [certificate,setCertificate]=useState<NativeAcademyCertificate|null>(null);
 const [status,setStatus]=useState("Откройте каталог, чтобы посмотреть доступные курсы.");
 const [busy,setBusy]=useState(false);
 const act=useCallback(async(work:()=>Promise<void>)=>{
  setBusy(true);
  try{await work()}catch(error){
   setStatus(error instanceof Error?error.message:"Образовательный сервис недоступен.");
  }finally{setBusy(false)}
 },[]);
 const loadLearning=useCallback(async(e:NativeAcademyEnrollment)=>{
  const data=await academyRequest<{lessons:NativeAcademyLesson[]}>(
   `/v1/academy/enrollments/${encodeURIComponent(e.id)}/learn`);
  setSelected(e);setLessons(data.lessons);setCertificate(null);
  setStatus("Ваш прогресс синхронизируется с APGIC.");
 },[]);
 return <View style={{gap:12,marginTop:16,padding:16,borderWidth:1,borderRadius:14}}
  accessibilityLabel="APGIC Academy">
  <Text style={{fontSize:22,fontWeight:"700"}}>Курсы и обучение</Text>
  <Text>Один учебный профиль на iOS, Android и Web. Сертификат не равен квалификации.</Text>
  <Text accessibilityRole="alert">{status}</Text>
  <Pressable accessibilityRole="button" disabled={busy}
   onPress={()=>void act(async()=>{
    const data=await academyRequest<{courses:NativeAcademyCourse[]}>("/v1/academy/courses");
    setCourses(data.courses);setStatus(data.courses.length?"Выберите курс.":"Опубликованных курсов нет.");
   })} style={{minHeight:48,borderWidth:1,borderRadius:9,padding:12}}>
   <Text>Каталог курсов</Text>
  </Pressable>
  <Pressable accessibilityRole="button" disabled={busy}
   onPress={()=>void act(async()=>{
    const data=await academyRequest<{enrollments:NativeAcademyEnrollment[]}>("/v1/academy/my-enrollments");
    setEnrollments(data.enrollments);setStatus(data.enrollments.length?"Продолжайте обучение.":"Зачислений пока нет.");
   })} style={{minHeight:48,borderWidth:1,borderRadius:9,padding:12}}>
   <Text>Мои обучения</Text>
  </Pressable>
  {courses.map(course=><View key={course.id} style={{gap:6,padding:8}}>
   <Text style={{fontWeight:"700"}}>{course.title}</Text>
   <Text>{course.description}</Text>
   <Text>Версия {course.version_no}</Text>
   <Pressable accessibilityRole="button" disabled={busy} style={{minHeight:48,justifyContent:"center"}}
    onPress={()=>void act(async()=>{
     const e=await academyRequest<NativeAcademyEnrollment>(
      `/v1/academy/courses/${encodeURIComponent(course.id)}/enroll`,"POST");
     await loadLearning(e);
    })}><Text>Открыть по подтверждённому праву</Text></Pressable>
  </View>)}
  {enrollments.map(item=><Pressable key={item.id} accessibilityRole="button" disabled={busy}
   style={{minHeight:48,justifyContent:"center"}}
   onPress={()=>void act(()=>loadLearning(item))}>
   <Text>Продолжить {item.id.slice(0,8)} · {item.state}</Text>
  </Pressable>)}
  {selected&&<View style={{gap:12}}>
   {lessons.map(lesson=><View key={lesson.id} style={{gap:8}}>
    <Text style={{fontWeight:"700"}}>{lesson.title}{lesson.completed?" ✓":""}</Text>
    {lesson.kind==="READING"&&<Text>{lesson.body}</Text>}
    {lesson.kind==="VIDEO"&&<Text>Видеоматериал требует подтверждённого media grant.</Text>}
    {lesson.kind==="ASSESSMENT"&&<Text>Письменное задание проходит проверку преподавателем. Для отправки используйте полную учебную форму на web.</Text>}
    {lesson.kind==="READING"&&!lesson.completed&&<Pressable accessibilityRole="button"
     disabled={busy} style={{minHeight:48,justifyContent:"center"}}
     onPress={()=>void act(async()=>{
      await academyRequest(
       `/v1/academy/enrollments/${encodeURIComponent(selected.id)}/lessons/${encodeURIComponent(lesson.id)}/complete`,"POST");
      await loadLearning(selected);
     })}><Text>Отметить прочитанным</Text></Pressable>}
   </View>)}
   <Pressable accessibilityRole="button" disabled={busy} style={{minHeight:48,justifyContent:"center"}}
    onPress={()=>void act(async()=>{
     const cert=await academyRequest<NativeAcademyCertificate>(
      `/v1/academy/enrollments/${encodeURIComponent(selected.id)}/complete`,"POST");
     setCertificate(cert);setStatus("Завершение зафиксировано в APGIC.");
    })}><Text>Завершить курс</Text></Pressable>
   {certificate&&<Text>Сертификат: {certificate.verification_url} · {certificate.evidence_candidate_state}</Text>}
  </View>}
 </View>;
}
