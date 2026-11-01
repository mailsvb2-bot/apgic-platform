"use client";

import {useState,type FormEvent} from "react";

type CloudMedia={id:string;title:string;provider:string;provider_file_ref:string;source_url?:string};
type VideoLesson={id:string;title:string;kind:string};
type Mapping={lesson_id:string;media_id:string;ordinal:number};
type AuthorMediaAssignment=Mapping&{title:string;provider:string;provider_file_ref:string;source_url?:string};

async function request<T>(url:string,body?:unknown):Promise<T>{
 const r=await fetch(url,{method:body===undefined?"GET":"POST",credentials:"same-origin",
  cache:"no-store",headers:{"Content-Type":"application/json"},
  body:body===undefined?undefined:JSON.stringify(body)});
 const data=await r.json().catch(()=>({}));
 if(!r.ok)throw new Error(data.message_safe||data.code||"Операция не выполнена");
 return data as T;
}
export default function CloudMediaEditor({courseId,versionId,lessons}:{
 courseId:string;versionId:string;lessons:VideoLesson[];
}){
 const [media,setMedia]=useState<CloudMedia[]>([]);
 const [rows,setRows]=useState("");
 const [selected,setSelected]=useState<string[]>([]);
 const [lessonId,setLessonId]=useState("");
 const [perLesson,setPerLesson]=useState(10);
 const [assignments,setAssignments]=useState<Mapping[]>([]);
 const [saved,setSaved]=useState<AuthorMediaAssignment[]>([]);
 const [message,setMessage]=useState("");
 const [busy,setBusy]=useState(false);
 async function reload(){
  const [d,a]=await Promise.all([
   request<{media:CloudMedia[]}>("/v1/academy/author/cloud-media"),
   request<{assignments:AuthorMediaAssignment[]}>(`/v1/academy/author/courses/${courseId}/versions/${versionId}/media`)
  ]);
  setMedia(d.media);setSaved(a.assignments);
 }
 async function importRows(e:FormEvent){
  e.preventDefault();setBusy(true);setMessage("");
  try{
   const lines=rows.trim().split(/\r?\n/).filter(Boolean);
   if(!lines.length||lines.length>2000)throw new Error("Введите от 1 до 2000 файлов");
   const items=lines.map(line=>{
    const parts=line.split("|");
    return {provider:"YANDEX_DISK",provider_file_ref:parts[0].trim(),title:parts.slice(1).join("|").trim()||parts[0].trim()};
   });
   if(items.some(x=>!x.provider_file_ref))throw new Error("Пустой идентификатор файла");
   await request("/v1/academy/author/cloud-media/import",{items});
   await reload();setRows("");setMessage("Видеозаписи добавлены в медиатеку");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
 function allocateAll(){
  const targets=lessons.filter(l=>l.kind==="VIDEO");
  const ids=selected.length?selected:media.map(m=>m.id);
  if(!targets.length||!ids.length){setMessage("Нужны видеоуроки и выбранные материалы");return}
  if(perLesson<1||perLesson>100||!Number.isInteger(perLesson)){setMessage("Видео на урок: от 1 до 100");return}
  if(ids.length>targets.length*perLesson){setMessage("Видео больше, чем свободных мест в выбранных уроках");return}
  const all=ids.map((id,i)=>({
   lesson_id:targets[Math.floor(i/perLesson)].id,
   media_id:id,ordinal:(i%perLesson)+1
  }));
  if(all.length>2000){setMessage("Разбейте импорт на группы до 2000 файлов");return}
  setAssignments(all);
  setMessage(`Подготовлено ${all.length} назначений для ${Math.ceil(all.length/perLesson)} уроков. Проверьте перед сохранением.`);
 }
 function allocate(){
  if(!lessonId||!selected.length)return;
  const known=assignments.filter(a=>a.lesson_id!==lessonId);
  const incoming=selected.map((id,i)=>({lesson_id:lessonId,media_id:id,ordinal:i+1}));
  setAssignments([...known,...incoming]);
  setMessage("Распределение подготовлено. Нажмите «Сохранить распределение»");
 }
 async function persist(){
  if(!assignments.length)return;
  setBusy(true);setMessage("");
  try{
   await request(`/v1/academy/courses/${courseId}/versions/${versionId}/media/assign`,{items:assignments});
   await reload();
   setMessage("Распределение сохранено");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
 return <section className="panel" aria-label="Массовое назначение видео">
  <h3>Медиатека автора</h3>
  <p>Импорт списка облачных файлов и распределение по видеоурокам. OAuth-выбор файлов в Яндекс Диске и ученическое воспроизведение пока не подключены.</p>
  <button type="button" disabled={busy} onClick={()=>void reload().catch(e=>setMessage(String(e)))}>Загрузить медиатеку</button>
  <p role="status">{message}</p>
  <form onSubmit={importRows}>
   <label htmlFor="cloud-media-rows">Список файлов: ID файла | название (каждый файл с новой строки)</label>
   <textarea id="cloud-media-rows" rows={5} value={rows} onChange={e=>setRows(e.target.value)} placeholder={"disk:/videos/001.mp4 | Видео 1\ndisk:/videos/002.mp4 | Видео 2"}/>
   <button disabled={busy}>Добавить файлы</button>
  </form>
  <h4>Выбрать сразу несколько видео</h4>
  <div style={{maxHeight:220,overflowY:"auto"}}>
   {media.map(m=><label key={m.id} style={{display:"block"}}>
    <input type="checkbox" checked={selected.includes(m.id)} onChange={e=>setSelected(old=>e.target.checked?[...old,m.id]:old.filter(id=>id!==m.id))}/>
    {m.title} · {m.provider_file_ref}
    {m.source_url&&<a href={m.source_url} rel="noreferrer" target="_blank">Оригинал</a>}
   </label>)}
  </div>
  <h4>Массовое автоматическое распределение</h4>
  <p>Выделите все видео (или оставьте список без выделения), выберите число видео на урок. Порядок определяется списком медиатеки и уроков.</p>
  <label htmlFor="cloud-per-lesson">Видео на урок</label>
  <input id="cloud-per-lesson" type="number" min={1} max={100} value={perLesson} onChange={e=>setPerLesson(Number(e.target.value))}/>
  <button type="button" disabled={busy||!media.length} onClick={allocateAll}>Распределить по всем видеоурокам</button>
  <label htmlFor="cloud-target-lesson">Или назначить выбранные видео одному уроку</label>
  <select id="cloud-target-lesson" value={lessonId} onChange={e=>setLessonId(e.target.value)}>
   <option value="">Выберите урок</option>
   {lessons.filter(l=>l.kind==="VIDEO").map(l=><option key={l.id} value={l.id}>{l.title}</option>)}
  </select>
  <button type="button" disabled={busy||!selected.length||!lessonId} onClick={allocate}>Назначить выбранные</button>
  <p>Подготовлено назначений: {assignments.length}</p>
  <div style={{maxHeight:220,overflowY:"auto"}}>
   {assignments.map((a,i)=><p key={a.lesson_id+":"+a.media_id+":"+i}>
    {lessons.find(l=>l.id===a.lesson_id)?.title||"Урок"} · {a.ordinal}. {media.find(m=>m.id===a.media_id)?.title||"Видео"}
   </p>)}
  </div>
  <h4>Сохранённые материалы уроков</h4>
  {saved.map(a=><p key={a.lesson_id+":"+a.media_id}>
    {lessons.find(l=>l.id===a.lesson_id)?.title||"Урок"} · {a.ordinal}. {a.title}
    {" · "}{a.provider_file_ref}
    {a.source_url&&<a href={a.source_url} target="_blank" rel="noreferrer">Оригинал</a>}
   </p>)}
  <button type="button" disabled={busy||!assignments.length} onClick={()=>void persist()}>Сохранить распределение</button>
 </section>;
}
