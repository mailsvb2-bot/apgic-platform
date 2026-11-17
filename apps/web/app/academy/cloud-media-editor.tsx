"use client";

import {useCallback,useEffect,useRef,useState,type FormEvent} from "react";
import {appendLessonMediaAssignments,moveLessonMediaAssignment,removeLessonMediaAssignment} from "./cloud-media-assignments";

type CloudMedia={id:string;title:string;provider:string;provider_file_ref:string;source_url?:string;active:boolean};
type DiskFile={name:string;path:string;type:string;mime_type:string;size:number};
type VideoLesson={id:string;title:string;kind:string;module_id:string};
type CourseModule={id:string;title:string;ordinal:number};
type Mapping={lesson_id:string;media_id:string;ordinal:number};
type AuthorMediaAssignment=Mapping&{title:string;provider:string;provider_file_ref:string;source_url?:string};

// Match the canonical provider's video verification before bulk selection.
// Non-video files must not invalidate a whole 100-file import batch.
function isImportableVideo(file:DiskFile):boolean{
 return file.type==="file"&&file.size>0&&file.mime_type.toLowerCase().startsWith("video/");
}

async function request<T>(url:string,body?:unknown,signal?:AbortSignal):Promise<T>{
 const r=await fetch(url,{method:body===undefined?"GET":"POST",credentials:"same-origin",
  cache:"no-store",signal,headers:{"Content-Type":"application/json"},
  body:body===undefined?undefined:JSON.stringify(body)});
 const data=await r.json().catch(()=>({}));
 if(!r.ok)throw new Error(data.message_safe||data.code||"Операция не выполнена");
 return data as T;
}
// The provider verifies each selected file on the server before saving it.
// Chunking preserves the existing 2,000-file author workflow while keeping
// each authenticated verification request bounded and safely retryable.
async function importVerifiedMedia(items:Array<{provider:string;provider_file_ref:string;title:string}>):Promise<CloudMedia[]>{
 const imported:CloudMedia[]=[];
 for(let i=0;i<items.length;i+=100){
  try{const batch=await request<{media:CloudMedia[]}>("/v1/academy/author/cloud-media/import",{items:items.slice(i,i+100)});imported.push(...batch.media);}
  catch(e){
   const reason=e instanceof Error?e.message:"Операция не выполнена";
   throw new Error(i>0?`Импортировано ${i} из ${items.length}. ${reason}`:reason);
  }
 }
 return imported;
}
export default function CloudMediaEditor({courseId,versionId,lessons,modules,focusedLessonId}:{
 courseId:string;versionId:string;lessons:VideoLesson[];modules:CourseModule[];focusedLessonId:string;
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
 const videoLessons=lessons.filter(l=>l.kind==="VIDEO");
 const [perLesson,setPerLesson]=useState(10);
 const [assignments,setAssignments]=useState<Mapping[]>([]);
 const [saved,setSaved]=useState<AuthorMediaAssignment[]>([]);
 const chosenLesson=videoLessons.find(l=>l.id===lessonId);
 const chosenModule=modules.find(m=>m.id===chosenLesson?.module_id);
 const currentLessonDraft=assignments.filter(a=>a.lesson_id===lessonId).sort((a,b)=>a.ordinal-b.ordinal);
 const currentLessonSaved=saved.filter(a=>a.lesson_id===lessonId);
 const [message,setMessage]=useState("");
 const [busy,setBusy]=useState(false);
 const oauthAutoOpened=useRef(false);
 // Choosing a VIDEO lesson in the parent outline opens the exact same editor.
 // This is presentation-only: all assignments remain owned by Academy APIs.
 useEffect(()=>{
  if(focusedLessonId&&videoLessons.some(l=>l.id===focusedLessonId)){
   setLessonId(focusedLessonId);setSelected([]);setDiskSelectedFiles({});
  }
 },[focusedLessonId,lessons]);
 async function browseDisk(path:string,offset=0){
  setBusy(true);setMessage("");
  try {
   const query=new URLSearchParams({path,offset:String(offset)});
   const result=await request<{files:DiskFile[];total:number;offset:number}>("/v1/academy/author/cloud/yandex/files?"+query);
   setDiskPath(path);setDiskFiles(result.files);setDiskTotal(result.total);setDiskOffset(offset);
  }catch(error){setMessage("Яндекс Диск: "+String(error))}
  finally{setBusy(false)}
 }
 // On OAuth return, show the real Disk folder without a second click.
 // Only the owner-signed callback can store the cloud credential; this
 // browser hint controls presentation, never access or authorization.
 useEffect(()=>{
  if(oauthAutoOpened.current)return;
  const query=new URLSearchParams(window.location.search);
  if(query.get("cloud_connected")!=="yandex")return;
  const qCourse=query.get("course_id"),qVersion=query.get("version_id"),qLesson=query.get("lesson_id");
  if((qCourse||qVersion)&&(qCourse!==courseId||qVersion!==versionId))return;
  // The OAuth round-trip returns to the exact video lesson. The parent
  // restores the draft first; wait for its outline before accepting that ID.
  if(qLesson&&lessons.length===0)return;
  if(qLesson){
   if(!videoLessons.some(lesson=>lesson.id===qLesson)){
    oauthAutoOpened.current=true;
    setMessage("Яндекс Диск подключён. Выберите актуальный видеоурок повторно.");
    return;
   }
   setLessonId(qLesson);
  }
  oauthAutoOpened.current=true;
  void browseDisk("disk:/");
 },[courseId,versionId,lessons]);
 async function importDiskSelection(){
  if(!Object.keys(diskSelectedFiles).length)return;
  setBusy(true);setMessage("");
  try{
   const chosen=Object.values(diskSelectedFiles).filter(isImportableVideo);
   if(!chosen.length||chosen.length!==Object.keys(diskSelectedFiles).length)throw new Error("Выбери только непустые видеофайлы");
   if(chosen.length>2000)throw new Error("За одну операцию можно добавить до 2000 файлов");
   const imported=await importVerifiedMedia(chosen.map(f=>({
    provider:"YANDEX_DISK",provider_file_ref:f.path,title:f.name
   })));
   if(imported.length!==chosen.length||imported.some(item=>!item.id))throw new Error("Импорт выполнен не полностью. Обновите медиатеку перед назначением.");
   await reload();
   setAssignments(current=>appendLessonMediaAssignments(current,lessonId,imported.map(item=>item.id)));
   setDiskSelectedFiles({});setMessage("Видео добавлены к выбранному уроку. Нажмите «Сохранить видео урока».");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
 const reload=useCallback(async (signal?:AbortSignal,restoreSavedDraft=false)=>{
  const [d,a]=await Promise.all([
   request<{media:CloudMedia[]}>("/v1/academy/author/cloud-media",undefined,signal),
   request<{assignments:AuthorMediaAssignment[]}>(`/v1/academy/author/courses/${courseId}/versions/${versionId}/media`,undefined,signal)
  ]);
  if(!signal?.aborted){
   // Stale references remain in author history but must not be offered for
   // new assignments after OAuth reconnect or file replacement.
   const activeMedia=d.media.filter(m=>m.active);
   const activeIDs=new Set(activeMedia.map(m=>m.id));
   setMedia(activeMedia);
   // A reconnect invalidates the old OAuth generation. Never leave stale
   // selections eligible for a new lesson mapping after the library reloads.
   setSelected(current=>current.filter(id=>activeIDs.has(id)));
   setDiskSelectedFiles({});
   setSaved(a.assignments);
   // The editor must reopen with the persisted mappings as its working
   // draft; otherwise saving one changed lesson silently replaces its
   // previously assigned videos with only the newly selected ones.
   if(restoreSavedDraft)setAssignments(a.assignments.map(({lesson_id,media_id,ordinal})=>({lesson_id,media_id,ordinal})));
  }
 },[courseId,versionId]);
 // Restore the author's saved library and lesson mappings on every editor
 // opening (including after the OAuth callback), without a manual refresh.
 useEffect(()=>{
  const controller=new AbortController();
  setMedia([]);setSaved([]);setAssignments([]);setSelected([]);
  // The editor is keyed by course/version, so a new editor already starts
  // with an empty lessonId. Do not clear it here: the OAuth-return effect
  // runs before this reload effect and restores the signed lesson selection.
  setDiskSelectedFiles({});
  void reload(controller.signal,true).catch(()=>{
   if(!controller.signal.aborted)setMessage("Не удалось загрузить медиатеку и сохранённые назначения. Повторите загрузку.");
  });
  return ()=>controller.abort();
 },[reload]);
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
  const activeIDs=new Set(media.map(m=>m.id));
  const ids=(selected.length?selected:media.map(m=>m.id)).filter(id=>activeIDs.has(id));
  if(!targets.length||!ids.length){setMessage("Нужны видеоуроки и выбранные материалы");return}
  if(perLesson<1||perLesson>100||!Number.isInteger(perLesson)){setMessage("Видео на урок: от 1 до 100");return}
  if(ids.length>targets.length*perLesson){setMessage("Видео больше, чем свободных мест в выбранных уроках");return}
  const groups=targets.map((lesson,i)=>({
   lessonId:lesson.id,mediaIds:ids.slice(i*perLesson,(i+1)*perLesson)
  })).filter(group=>group.mediaIds.length>0);
  let result:Mapping[]=assignments;
  for(const group of groups)result=appendLessonMediaAssignments(result,group.lessonId,group.mediaIds);
  if(result.length>2000){setMessage("Разбейте распределение на группы до 2000 назначений");return}
  const added=result.length-assignments.length;
  setAssignments(result);
  setMessage(`Добавлено ${added} видео к ${groups.length} урокам без удаления существующих привязок. Проверьте и сохраните.`);
 }
 function allocate(){
  if(!lessonId||!selected.length)return;
  const activeIDs=new Set(media.map(m=>m.id));
  if(selected.some(id=>!activeIDs.has(id))){
   setMessage("Выбранные видео устарели после переподключения облака. Обновите медиатеку.");return;
  }
  setAssignments(current=>appendLessonMediaAssignments(current,lessonId,selected));
  setMessage("Видео добавлены к уроку без удаления предыдущих. Нажмите «Сохранить видео урока».");
 }
 async function persist(){
  if(!assignments.length&&!saved.length)return;
  setBusy(true);setMessage("");
  try{
   const activeIDs=new Set(media.map(m=>m.id));
   if(assignments.some(a=>!activeIDs.has(a.media_id))){
    throw new Error("В распределении есть видео от прежнего подключения. Удалите устаревшие назначения и импортируйте актуальные файлы.");
   }
   const source=assignments.reduce((map,a)=>{const values=map.get(a.lesson_id)||[];values.push(a);map.set(a.lesson_id,values);return map},new Map<string,Mapping[]>());
   if([...source.values()].some(rows=>new Set(rows.map(x=>x.media_id)).size!==rows.length))throw new Error("Одно видео назначено уроку дважды");
   await request(`/v1/academy/courses/${courseId}/versions/${versionId}/media/assign`,{items:assignments});
   await reload(undefined,true);
   setMessage("Распределение сохранено");
  }catch(error){setMessage(String(error))}finally{setBusy(false)}
 }
 return <section id="academy-media-editor" className="panel" aria-label="Медиатека автора">
  <h3>Шаг 3. Прикрепить видео к уроку</h3>
  <p>Курс → модуль → видеоурок → видеофайл → сохранение. Яндекс Диск — источник файлов, а доступ ученика проверяет APGIC.</p>
  {videoLessons.length===0?<p role="status">В этом курсе пока нет видеоуроков. Сначала создайте модуль, затем урок типа «Видеоурок» в шаге 2. Пока прикреплять видео не к чему.</p>:<>
  <label htmlFor="academy-video-lesson">К какому уроку добавить видео?</label>
  <select id="academy-video-lesson" value={lessonId} onChange={e=>{setLessonId(e.target.value);setSelected([]);setDiskSelectedFiles({});}} disabled={busy}>
   <option value="">Выберите модуль и видеоурок</option>
   {modules.map(module=><optgroup key={module.id} label={`${module.ordinal}. ${module.title}`}>
    {videoLessons.filter(l=>l.module_id===module.id).map(l=><option key={l.id} value={l.id}>{l.title}</option>)}
   </optgroup>)}
  </select>
  {!lessonId?<p role="status">Выберите конкретный видеоурок в его модуле. Массовое распределение не требуется.</p>:<>
  <p role="status"><strong>Редактируем:</strong> {chosenModule?.title||"Модуль"} → {chosenLesson?.title||"Видеоурок"}</p>
  <h4>Прикреплённые видео этого урока</h4>
  {currentLessonDraft.length===0?<p>Пока нет видео. Выберите один или несколько файлов на Яндекс Диске ниже.</p>:
   <ol>{currentLessonDraft.map((a,i)=><li key={a.media_id}>
    {media.find(m=>m.id===a.media_id)?.title||"Видеофайл"}{currentLessonSaved.some(v=>v.media_id===a.media_id&&v.ordinal===a.ordinal)?" · сохранено":" · не сохранено"}
    {" "}<button type="button" disabled={busy||i===0} onClick={()=>setAssignments(old=>moveLessonMediaAssignment(old,lessonId,a.media_id,-1))}>Выше</button>
    {" "}<button type="button" disabled={busy||i===currentLessonDraft.length-1} onClick={()=>setAssignments(old=>moveLessonMediaAssignment(old,lessonId,a.media_id,1))}>Ниже</button>
    {" "}<button type="button" disabled={busy} onClick={()=>setAssignments(old=>removeLessonMediaAssignment(old,lessonId,a.media_id))}>Убрать</button>
   </li>)}</ol>}
  <h4>Уже добавленные видео в медиатеке</h4>
  <p>Выберите одно или несколько видео для этого урока. Повторно загружать их с Яндекс Диска не нужно.</p>
  {media.length===0?<p role="status">Медиатека пока пуста. Вы можете открыть Яндекс Диск ниже.</p>:
   <div style={{maxHeight:240,overflowY:"auto"}}>
    {media.map(m=><label key={m.id} style={{display:"block"}}>
     <input type="checkbox" checked={selected.includes(m.id)}
      disabled={busy||currentLessonDraft.some(a=>a.media_id===m.id)}
      onChange={e=>setSelected(old=>e.target.checked?[...old,m.id]:old.filter(id=>id!==m.id))}/>
     {m.title}{currentLessonDraft.some(a=>a.media_id===m.id)?" · уже в уроке":""}
    </label>)}
   </div>}
  <button type="button" disabled={busy||selected.length===0} onClick={allocate}>
   Прикрепить выбранные видео ({selected.length})
  </button>
  <details><summary>Добавить новые файлы с Яндекс Диска</summary>
  <h4>Ваш Яндекс Диск</h4>
  <a href={`/v1/academy/author/cloud/yandex/connect?course_id=${encodeURIComponent(courseId)}&version_id=${encodeURIComponent(versionId)}&lesson_id=${encodeURIComponent(lessonId)}`}>Подключить Яндекс Диск</a>
  <p>После подтверждения в Яндексе файлы откроются автоматически. Выберите нужные видео.</p>
  <button type="button" disabled={busy} onClick={()=>void browseDisk("disk:/")}>Открыть Яндекс Диск</button>
  <p>Папка: {diskPath}</p>
  {diskPath!=="disk:/"&&<button type="button" disabled={busy} onClick={()=>void browseDisk(diskPath.slice(0,diskPath.lastIndexOf("/"))==="disk:"?"disk:/":diskPath.slice(0,diskPath.lastIndexOf("/")))}>На уровень выше</button>}
  <div style={{maxHeight:240,overflowY:"auto"}}>
   {diskFiles.map(file=><div key={file.path}>
    {file.type==="dir"
      ?<button type="button" disabled={busy} onClick={()=>void browseDisk(file.path)}>Папка: {file.name}</button>
      :isImportableVideo(file)
      ? <label><input type="checkbox" checked={Boolean(diskSelectedFiles[file.path])}
        onChange={e=>{setDiskSelectedFiles(old=>{const next={...old};if(e.target.checked)next[file.path]=file;else delete next[file.path];return next;});}}/>
        {file.name}</label>
      : <span>{file.name} · не является доступным видеофайлом</span>}
   </div>)}
  </div>
  {diskTotal>100&&<div>
   <button type="button" disabled={busy||diskOffset===0} onClick={()=>void browseDisk(diskPath,Math.max(0,diskOffset-100))}>Назад</button>
   <span>{diskOffset+1}–{Math.min(diskOffset+100,diskTotal)} из {diskTotal}</span>
   <button type="button" disabled={busy||diskOffset+100>=diskTotal} onClick={()=>void browseDisk(diskPath,diskOffset+100)}>Далее</button>
  </div>}
  <button type="button" disabled={busy||!lessonId||!Object.keys(diskSelectedFiles).length} onClick={()=>void importDiskSelection()}>Добавить к уроку ({Object.keys(diskSelectedFiles).length})</button>
  </details>
  <p role="status" aria-live="polite">{message}</p>
  <details><summary>Дополнительно: импорт по путям файлов</summary><form onSubmit={importRows}>
   <label htmlFor="cloud-media-rows">Список файлов: ID файла | название (каждый файл с новой строки)</label>
   <textarea id="cloud-media-rows" rows={5} value={rows} onChange={e=>setRows(e.target.value)} placeholder={"disk:/videos/001.mp4 | Видео 1\ndisk:/videos/002.mp4 | Видео 2"}/>
   <button disabled={busy}>Добавить файлы</button>
  </form></details>
  {saved.some(a=>!media.some(m=>m.id===a.media_id))&&<div role="alert">
   <p>В сохранённых уроках есть видео от прежнего подключения Яндекс Диска. Они недоступны ученикам. Повторно импортируйте актуальные файлы и переназначьте их перед публикацией.</p>
   {assignments.some(a=>!media.some(m=>m.id===a.media_id))&&<button type="button" disabled={busy} onClick={()=>{
    const activeIDs=new Set(media.map(m=>m.id));
    setAssignments(current=>current.filter(a=>activeIDs.has(a.media_id)));
    setMessage("Устаревшие назначения убраны из черновика. Нажмите «Сохранить распределение», чтобы применить изменения.");
   }}>Убрать все устаревшие назначения</button>}
  </div>}
  <details><summary>Дополнительно: массово распределить видео по урокам</summary>
  <h4>Массовое автоматическое распределение</h4>
  <p>Выделите все видео (или оставьте список без выделения), выберите число видео на урок. Порядок определяется списком медиатеки и уроков.</p>
  <label htmlFor="cloud-per-lesson">Видео на урок</label>
  <input id="cloud-per-lesson" type="number" min={1} max={100} value={perLesson} onChange={e=>setPerLesson(Number(e.target.value))}/>
  <button type="button" disabled={busy||!media.length} onClick={allocateAll}>Распределить по всем видеоурокам</button>
  <p>Подготовлено назначений: {assignments.length}. Сохранённые назначения восстанавливаются при повторном входе; изменения вступят в силу только после сохранения. Удаление всех назначений очищает видеопривязки черновика.</p>
  <button type="button" disabled={busy} onClick={()=>{
   setAssignments(saved.map(({lesson_id,media_id,ordinal})=>({lesson_id,media_id,ordinal})));
   setMessage("Несохранённые изменения отменены");
  }}>Отменить изменения распределения</button>
  <div style={{maxHeight:220,overflowY:"auto"}}>
   {assignments.map((a,i)=><p key={a.lesson_id+":"+a.media_id+":"+i}>
    {lessons.find(l=>l.id===a.lesson_id)?.title||"Урок"} · {a.ordinal}. {media.find(m=>m.id===a.media_id)?.title||"Видео"}
    <button type="button" disabled={busy} onClick={()=>setAssignments(old=>old.filter((_,index)=>index!==i))}>Убрать из урока</button>
   </p>)}
  </div>
  </details>
  <details><summary>Сохранённые назначения всех уроков</summary>
   {saved.length===0?<p>Назначений пока нет.</p>:saved.map(a=><p key={a.lesson_id+":"+a.media_id}>{lessons.find(l=>l.id===a.lesson_id)?.title||"Урок"} · {a.ordinal}. {a.title}</p>)}
  </details>
  <button type="button" disabled={busy||(!assignments.length&&!saved.length)} onClick={()=>void persist()}>Сохранить видео урока</button>
  </>}
  </>}
 </section>;
}
