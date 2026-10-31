"use client";

import { useEffect, useState } from "react";
import type {ConsultationTimeline as Timeline} from "../../../packages/contracts/src/generated/apgic-v1";
const labels:Record<string,string> = {
  JOINED:"Подключение подтверждено провайдером", STARTED:"Медиасеанс начат",
  LEFT:"Отключение участника", RECOVERY_STARTED:"Восстановление связи",
  RECOVERY_SUCCEEDED:"Связь восстановлена",
  TECHNICAL_FAILURE:"Технический сбой подтверждён",ENDED:"Сеанс завершён провайдером",
};
function validTimeline(value:unknown,id:string):value is Timeline {
  if (!value || typeof value!=="object")return false;
  const v=value as Partial<Timeline>;
  return v.booking_id===id && typeof v.booking_state==="string" &&
    typeof v.consultation_state==="string" && Array.isArray(v.facts) &&
    Array.isArray(v.recovery) && typeof v.requires_followup==="boolean" &&
    v.facts.every(x=>x && typeof x.fact_type==="string" &&
      typeof x.role==="string" && typeof x.evidence_ref==="string" &&
      Number.isFinite(Date.parse(x.occurred_at))) &&
    v.recovery.every(x=>x && typeof x.action==="string" &&
      typeof x.reason_code==="string" && Number.isFinite(Date.parse(x.decided_at)));
}
export default function ConsultationHistory({bookingID}:{bookingID:string}) {
  const [timeline,setTimeline]=useState<Timeline|null>(null);
  const [status,setStatus]=useState<"IDLE"|"CHECKING"|"DENIED"|"ABSENT"|"ERROR">("IDLE");
  useEffect(()=>{setTimeline(null);setStatus("IDLE")},[bookingID]);
  async function refresh(){
    setStatus("CHECKING");
    try{
      const response=await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/timeline`,{
        credentials:"include",cache:"no-store",headers:{Accept:"application/json"},
      });
      if(response.status===401||response.status===403){setTimeline(null);setStatus("DENIED");return}
      if(response.status===404){setTimeline(null);setStatus("ABSENT");return}
      if(!response.ok){setTimeline(null);setStatus("ERROR");return}
      const payload:unknown=await response.json();
      if(!validTimeline(payload,bookingID)){setTimeline(null);setStatus("ERROR");return}
      setTimeline(payload);setStatus("IDLE");
    }catch{setTimeline(null);setStatus("ERROR")}
  }
  return <section className="journey-stage" aria-label="Хронология сеанса">
    <h3>История проведения консультации</h3>
    <p>Факты провайдера и решения о восстановлении из PostgreSQL. Браузер не устанавливает итог самостоятельно.</p>
    <button type="button" disabled={status==="CHECKING"} onClick={()=>void refresh()}>
      {status==="CHECKING"?"Читаем историю…":"Проверить историю сеанса"}
    </button>
    {status==="DENIED"?<p role="alert">Для просмотра истории требуется повторный вход.</p>:null}
    {status==="ABSENT"?<p role="status">Подтверждённой истории медиасеанса пока нет.</p>:null}
    {status==="ERROR"?<p role="alert">Не удалось загрузить историю из PostgreSQL.</p>:null}
    {timeline?<div aria-live="polite">
      <p>Статус сеанса: <strong>{timeline.consultation_state}</strong>. Статус записи: {timeline.booking_state}.</p>
      {timeline.requires_followup?<p role="alert">Зафиксирован технический сбой. Потребуется подтверждённое решение о переносе или возврате через владельца бронирования и платежей; повторная оплата автоматически не требуется.</p>:null}
      <ol>{timeline.facts.map((f,i)=><li key={`${i}-${f.fact_type}-${f.occurred_at}`}>
        {new Date(f.occurred_at).toLocaleString("ru-RU")}: {labels[f.fact_type]??f.fact_type}
        {f.role!=="SYSTEM"?` (${f.role==="CLIENT"?"клиент":"специалист"})`:""}
      </li>)}</ol>
      {timeline.recovery.length>0?<div><h4>Решения после сбоев</h4>
        <ul>{timeline.recovery.map((r,i)=><li key={`${i}-${r.decided_at}`}>
          {new Date(r.decided_at).toLocaleString("ru-RU")}: {r.action} — {r.reason_code}
          {r.followup_path_ref?` (${r.followup_path_ref})`:""}
        </li>)}</ul></div>:null}
    </div>:null}
  </section>;
}
