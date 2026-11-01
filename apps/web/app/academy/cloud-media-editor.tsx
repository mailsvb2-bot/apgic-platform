"use client";

import {useState,type FormEvent} from "react";

type CloudMedia={id:string;title:string;provider:string;provider_file_ref:string;source_url?:string};
type DiskFile={name:string;path:string;type:string;mime_type:string;size:number};
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
// The provider verifies each selected file on the server before saving it.
// Chunking preserves the existing 2,000-file author workflow while keeping
// each authenticated verification request bounded and safely retryable.
async function importVerifiedMedia(items:Array<{provider:string;provider_file_ref:string;title:string}>){
 for(let i=0;i<items.length;i+=100){
  await request("/v1/academy/author/cloud-media/import",{items:items.slice(i,i+100)});
 }
}
export default function CloudMediaEditor({courseId,versionId,lessons}:{
 courseId:string;versionId:string;lessons:VideoLesson[];
}){
 const [media,setMedia]=useState<CloudMedia[]>([]);
 const [diskPath,setDiskPath]=useState("disk:/");
 const [diskFiles,setDiskFiles]=useState<DiskFile[]>([]);
 const [diskOffset,setDiskOffset]=useState(0);
 const [diskTotal,setDiskTotal]=useState(0);
 const [diskSelectedFiles,setDiskSelectedFiles]=useState<Record<string,DiskFile>>({});
 const [rows,setRows]=useState("");
 const [selected,setSelected]=useState<string[]>([]);
 const [lessonId,setLessonId]=useState("");
 const [perLesson,setPerLesson]=useState(10);
 const [assignments,setAssignments]=useState<Mapping[]>([]);
 const [saved,setSaved]=useState<AuthorMediaAssignment[]>([]);
 const [message,setMessage]=useState("");
 const [busy,setBusy]=useState(false);
 async function browseDisk(path:string,offset=0){
  setBusy(true);setMessage("");
  try {
   const query=new URLSearchParams({path,offset:String(offset)});
   const result=await request<{files:DiskFile[];total:number;offset:number}>("/v1/academy/author/cloud/yandex/files?"+query);
   setDiskPath(path);setDiskFiles(result.files);setDiskTotal(result.total);setDiskOffset(offset);
  }catch(error){setMessage("Яндекс Диск: "+String(error))}
  finally{setBusy(false)}
 }
 async function importDiskSelection(){
  if(!Object.keys(diskSelectedFiles).length)return;
  setBusy(true);setMessage("");
  try{
   const chosen=Object.values(diskSelectedFiles).filter(f=>f.type==="file");
   if(!chosen.length)throw new Error("Выбери видеофайлы");
   if(chosen.length>2000)throw new Error("За одну операцию можно добавить до 2000 файлов");
   await importVerifiedMedia(chosen.map(f=>({
    provider:"YANDEX_DISK",provider_file_ref:f.path,title:f.name
   })));
   await reload();setDiskSelectedFiles({});setMessage("Выбранные видео добавлены в медиатеку");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
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
   await importVerifiedMedia(items);
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
   const source=assignments.reduce((map,a)=>{const values=map.get(a.lesson_id)||[];values.push(a);map.set(a.lesson_id,values);return map},new Map<string,Mapping[]>());
   if([...source.values()].some(rows=>new Set(rows.map(x=>x.media_id)).size!==rows.length))throw new Error("Одно видео назначено уроку дважды");
   await request(`/v1/academy/courses/${courseId}/versions/${versionId}/media/assign`,{items:assignments});
   await reload();
   setMessage("Распределение сохранено");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
 return <section className="panel" aria-label="Массовое назначение видео">
  <h3>Медиатека автора</h3>
  <p>Импорт облачных файлов и распределение по видеоурокам. Воспроизведение доступно только после проверки доступа через серверный медиашлюз.</p>
  <button type="button" disabled={busy} onClick={()=>void reload().catch(e=>setMessage(String(e)))}>Загрузить медиатеку</button>
  <h4>Выбрать файлы из Яндекс Диска</h4>
  <a href="/v1/academy/author/cloud/yandex/connect">Подключить Яндекс Диск</a>
  <p>После подключения облака можно открыть папку, отметить сразу несколько видео и добавить их одним нажатием.</p>
  <button type="button" disabled={busy} onClick={()=>void browseDisk("disk:/")}>Открыть Яндекс Диск</button>
  <p>Папка: {diskPath}</p>
  {diskPath!=="disk:/"&&<button type="button" disabled={busy} onClick={()=>void browseDisk(diskPath.slice(0,diskPath.lastIndexOf("/"))==="disk:"?"disk:/":diskPath.slice(0,diskPath.lastIndexOf("/")))}>На уровень выше</button>}
  <div style={{maxHeight:240,overflowY:"auto"}}>
   {diskFiles.map(file=><div key={file.path}>
    {file.type==="dir"
      ?<button type="button" disabled={busy} onClick={()=>void browseDisk(file.path)}>Папка: {file.name}</button>
      :<label><input type="checkbox" checked={Boolean(diskSelectedFiles[file.path])}
        onChange={e=>{setDiskSelectedFiles(old=>{const next={...old};if(e.target.checked)next[file.path]=file;else delete next[file.path];return next;});}}/>
        {file.name}</label>}
   </div>)}
  </div>
  {diskTotal>100&&<div>
   <button type="button" disabled={busy||diskOffset===0} onClick={()=>void browseDisk(diskPath,Math.max(0,diskOffset-100))}>Назад</button>
   <span>{diskOffset+1}–{Math.min(diskOffset+100,diskTotal)} из {diskTotal}</span>
   <button type="button" disabled={busy||diskOffset+100>=diskTotal} onClick={()=>void browseDisk(diskPath,diskOffset+100)}>Далее</button>
  </div>}
  <button type="button" disabled={busy||!Object.keys(diskSelectedFiles).length} onClick={()=>void importDiskSelection()}>Добавить выбранные видео ({Object.keys(diskSelectedFiles).length})</button>
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
