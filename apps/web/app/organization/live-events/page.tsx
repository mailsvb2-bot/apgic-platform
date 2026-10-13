"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import type {LiveAttendanceSummary} from "../../../../../packages/contracts/src/r9-live-events";

type MediaGrant={join_url:string;expires_at:string;role:string};
type Organization = { id: string; name: string };
type EventState = "DRAFT" | "PUBLISHED" | "LIVE" | "COMPLETED" | "CANCELLED";
type LiveEvent = {
  id: string;
  organization_id: string;
  title: string;
  state: EventState;
  starts_at: string;
  ends_at: string;
  capacity: number;
  registered: number;
  recording_available: boolean;
};
const transitions: Partial<Record<EventState, { action: string; label: string }>> = {
  DRAFT: { action: "publish", label: "Опубликовать" },
  PUBLISHED: { action: "start", label: "Начать эфир" },
  LIVE: { action: "complete", label: "Завершить" },
};
async function request<T>(url: string, init: RequestInit = {}): Promise<T> {
  const result = await fetch(url, { cache: "no-store", credentials: "include", ...init });
  const payload = await result.json().catch(() => ({}));
  if (!result.ok) throw new Error(payload?.message_safe || payload?.error?.message || `Ошибка ${result.status}`);
  return payload as T;
}
export default function ManageLiveEvents() {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [org, setOrg] = useState("");
  const [items, setItems] = useState<LiveEvent[]>([]);
  const [attendance, setAttendance] = useState<Record<string, LiveAttendanceSummary>>({});
  const [mediaPrepared, setMediaPrepared] = useState<Record<string, boolean>>({});
  const [hostGrants, setHostGrants] = useState<Record<string, MediaGrant>>({});
  const [participants, setParticipants] = useState<Record<string, string[]>>({});
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [format, setFormat] = useState("WEBINAR");
  const [start, setStart] = useState("");
  const [duration, setDuration] = useState(60);
  const [capacity, setCapacity] = useState(25);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    request<{ organizations: Organization[] }>("/v1/organizations")
      .then(r => { setOrganizations(r.organizations || []); if (r.organizations?.[0]) setOrg(r.organizations[0].id); })
      .catch(() => setError("Для создания мероприятия нужен вход и своя организация."));
  }, []);
  const refresh = useCallback(async () => {
    if (!org) { setItems([]); return; }
    const data = await request<{ events: LiveEvent[] }>(`/v1/organizations/${encodeURIComponent(org)}/live-events`);
    setItems(data.events || []);
  }, [org]);
  useEffect(() => { void refresh().catch(() => setError("Список мероприятий сейчас недоступен.")); }, [refresh]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const startDate = new Date(start);
      if (!Number.isFinite(startDate.getTime()) || !Number.isFinite(duration) || duration < 15 || duration > 480)
        throw new Error("Проверьте дату и продолжительность.");
      const data = {
        title, description, format,
        starts_at: startDate.toISOString(),
        ends_at: new Date(startDate.getTime() + duration * 60000).toISOString(),
        capacity, price_minor: 0, currency: "RUB",
      };
      const result = await request<LiveEvent>(`/v1/organizations/${encodeURIComponent(org)}/live-events`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() + crypto.randomUUID() },
        body: JSON.stringify(data),
      });
      setNotice(`Черновик «${result.title}» создан и сохранён.`);
      await refresh();
    } catch (e) { setError(e instanceof Error ? e.message : "Не удалось создать мероприятие."); }
    finally { setBusy(false); }
  }
  async function prepareMedia(item:LiveEvent) {
    setBusy(true);setError("");setNotice("");
    try{
      const response=await request<{event_id:string;provider_ready:boolean;conference_bound:boolean}>(
        `/v1/live-events/${encodeURIComponent(item.id)}/prepare-media`,{method:"POST"});
      if(!response.provider_ready||!response.conference_bound)throw new Error("UCR не подтвердил готовность комнаты.");
      setMediaPrepared(x=>({...x,[item.id]:true}));
      setNotice("UCR-комната создана и проверена. Теперь можно управлять мероприятием.");
    }catch(e){setError(e instanceof Error?e.message:"Видеопровайдер недоступен.")}
    finally{setBusy(false)}
  }
  async function hostJoin(item:LiveEvent) {
    setBusy(true);setError("");
    try{
      const grant=await request<MediaGrant>(`/v1/live-events/${encodeURIComponent(item.id)}/join`,{method:"POST"});
      if(!grant.join_url)throw new Error("UCR не выдал защищённый доступ.");
      setHostGrants(x=>({...x,[item.id]:grant}));
    }catch(e){setError(e instanceof Error?e.message:"Сейчас невозможно подключиться.")}
    finally{setBusy(false)}
  }
  async function loadParticipants(item:LiveEvent){
    setBusy(true);setError("");
    try{
      const results=await request<{identities:string[]}>(
       `/v1/live-events/${encodeURIComponent(item.id)}/participants`);
      setParticipants(x=>({...x,[item.id]:results.identities||[]}));
    }catch(e){setError(e instanceof Error?e.message:"Список участников недоступен.")}
    finally{setBusy(false)}
  }
  async function moderate(item:LiveEvent,identity:string,action:"mute"|"remove"){
    setBusy(true);setError("");
    try{
      await request(`/v1/live-events/${encodeURIComponent(item.id)}/moderate`,{
       method:"POST",headers:{"Content-Type":"application/json"},
       body:JSON.stringify({identity_id:identity,action})
      });
      setNotice(action==="mute"?"UCR подтвердил отключение микрофона.":"UCR удалил участника из комнаты; повторный доступ заблокирован.");
    }catch(e){setError(e instanceof Error?e.message:"Не удалось выполнить модерацию.")}
    finally{setBusy(false)}
  }
  async function syncVerifiedAttendance(item:LiveEvent) {
    setBusy(true);setError("");
    try{
      const result=await request<{registered:number;provider_verified_attendees:number}>(
        `/v1/live-events/${encodeURIComponent(item.id)}/attendance/sync`,{method:"POST"});
      setNotice(`Подтверждено UCR: ${result.provider_verified_attendees} посещений из ${result.registered} регистраций.`);
      const stats=await request<LiveAttendanceSummary>(
        `/v1/organizations/${encodeURIComponent(org)}/live-events/${encodeURIComponent(item.id)}/attendance`);
      setAttendance(x=>({...x,[item.id]:stats}));
    }catch(e){setError(e instanceof Error?e.message:"Не удалось подтвердить посещаемость.")}
    finally{setBusy(false)}
  }
  async function inspectAttendance(item: LiveEvent) {
    setError("");
    setBusy(true);
    try {
      const result = await request<LiveAttendanceSummary>(`/v1/organizations/${encodeURIComponent(org)}/live-events/${encodeURIComponent(item.id)}/attendance`);
      setAttendance(current => ({ ...current, [item.id]: result }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Статистика подтверждённых посещений недоступна.");
    } finally { setBusy(false); }
  }
  async function change(item: LiveEvent, action: string) {
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const updated = await request<LiveEvent>(`/v1/live-events/${encodeURIComponent(item.id)}/${action}`, { method: "POST" });
      setNotice(`«${updated.title}»: состояние ${updated.state} подтверждено сервером.`);
      await refresh();
    } catch (e) { setError(e instanceof Error ? e.message : "Проверьте состояние и готовность видеосервиса."); }
    finally { setBusy(false); }
  }

  return <main className="site-shell">
    <header className="site-header">
      <Link className="brand" href="/"><span className="brand-mark">A</span> APGIC</Link>
      <nav className="top-nav" aria-label="Разделы">
        <Link href="/organization">Организация</Link>
        <Link href="/live-events">Афиша</Link>
      </nav>
    </header>
    <section className="live-events-panel">
      <p className="section-kicker">Организатору</p>
      <h1>Групповые мероприятия</h1>
      <p>Встречи принадлежат организации. Публикация требует подтверждённого SFU-провайдера;
        платные мероприятия пока недоступны, чтобы не создавать фиктивные платежи.</p>
      {organizations.length > 0 && <label className="live-org-selector">Организация
        <select value={org} onChange={e => setOrg(e.target.value)}>
          {organizations.map(x => <option key={x.id} value={x.id}>{x.name}</option>)}
        </select>
      </label>}
      <h2>Ваши мероприятия</h2>
      {items.length === 0 && <p>Пока нет сохранённых мероприятий.</p>}
      <ul className="live-events-grid">{items.map(item => {
        const next = transitions[item.state];
        return <li key={item.id} className="live-event-card">
          <h3><Link href={`/live-events/${encodeURIComponent(item.id)}`}>{item.title}</Link></h3>
          <p>Состояние: <strong>{item.state}</strong></p>
          <p>Начало: {new Date(item.starts_at).toLocaleString("ru-RU")}</p>
          <p>Зарегистрировано: {item.registered} из {item.capacity}</p>
          <div className="live-action-row">
            {(item.state==="DRAFT"||item.state==="PUBLISHED"||item.state==="LIVE") &&
             <button type="button" disabled={busy} onClick={()=>void prepareMedia(item)}>Проверить и привязать UCR-комнату</button>}
            {(item.state==="PUBLISHED"||item.state==="LIVE") &&
             <button type="button" disabled={busy} onClick={()=>void hostJoin(item)}>Войти ведущим</button>}
            {(item.state==="PUBLISHED"||item.state==="LIVE"||item.state==="COMPLETED") &&
             <button type="button" disabled={busy} onClick={()=>void syncVerifiedAttendance(item)}>Обновить посещаемость из UCR</button>}
            {(item.state==="PUBLISHED"||item.state==="LIVE") &&
             <button type="button" disabled={busy} onClick={()=>void loadParticipants(item)}>Управление участниками</button>}
            {next && <button type="button" disabled={busy} onClick={() => void change(item, next.action)}>{next.label}</button>}
            <button type="button" disabled={busy} onClick={() => void inspectAttendance(item)}>Посещаемость</button>
            {(item.state === "DRAFT" || item.state === "PUBLISHED" || item.state === "LIVE") &&
              <button type="button" disabled={busy} onClick={() => void change(item, "cancel")}>Отменить мероприятие</button>}
          </div>
          {participants[item.id]&&<div aria-label="Модерация участников">
            <p>Регистраций в списке: {participants[item.id].length}</p>
            {participants[item.id].map(identity=><div key={identity} className="live-action-row">
             <code>{identity}</code>
             <button type="button" disabled={busy} onClick={()=>void moderate(item,identity,"mute")}>Отключить микрофон</button>
             <button type="button" disabled={busy} onClick={()=>void moderate(item,identity,"remove")}>Удалить из эфира</button>
            </div>)}
           </div>}
          {mediaPrepared[item.id]&&<p role="status">UCR-медиасервис подтвердил комнату.</p>}
          {hostGrants[item.id]&&<p role="status">
            <a href={hostGrants[item.id].join_url} target="_blank" rel="noopener noreferrer">
              Открыть групповую медиасессию ведущего
            </a>
          </p>}
          {attendance[item.id] && <p role="status">Подтверждённое посещение: {attendance[item.id].joined_participants} участников из {attendance[item.id].registered} регистраций.
            Источник: подписанные события или подтверждённая машинная API-сессия UCR; это не протокол видеозаписи.</p>}
        </li>;
      })}</ul>
      <h2>Создать мероприятие</h2>
      <form className="live-form" onSubmit={e => void submit(e)}>
        <label>Название<input required minLength={5} maxLength={160} value={title} onChange={e => setTitle(e.target.value)} /></label>
        <label>Описание<textarea maxLength={4000} value={description} onChange={e => setDescription(e.target.value)} /></label>
        <label>Формат<select value={format} onChange={e => setFormat(e.target.value)}>
          <option value="WEBINAR">Вебинар</option>
          <option value="GROUP">Групповая встреча</option>
          <option value="WORKSHOP">Мастер-класс</option>
          <option value="QA">Вопросы и ответы</option>
          <option value="CONFERENCE">Конференция</option>
        </select></label>
        <label>Начало<input required type="datetime-local" value={start} onChange={e => setStart(e.target.value)} /></label>
        <label>Продолжительность, минут<input type="number" min={15} max={480} required value={duration} onChange={e => setDuration(Number(e.target.value))} /></label>
        <label>Мест<input type="number" min={2} max={1000} required value={capacity} onChange={e => setCapacity(Number(e.target.value))} /></label>
        <button disabled={busy || !org} type="submit">Создать черновик</button>
      </form>
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  </main>;
}
