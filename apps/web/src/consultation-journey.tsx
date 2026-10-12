"use client";

import { useEffect, useRef, useState } from "react";
import type {
  ConsultationReadySnapshot as Readiness,
  ConsultationClientOutcome as Outcome,
  ConsultationJoinGrant,
} from "../../../packages/contracts/src/generated/apgic-v1";

function validReady(payload: unknown, id: string): payload is Readiness {
  if (typeof payload !== "object" || !payload) return false;
  const x = payload as Partial<Readiness>;
  return x.booking_id === id && (x.role === "CLIENT" || x.role === "SPECIALIST") &&
    typeof x.booking_state === "string" && typeof x.opens_at === "string" &&
    typeof x.closes_at === "string" &&
    Number.isFinite(Date.parse(x.opens_at)) && Number.isFinite(Date.parse(x.closes_at));
}

export default function ConsultationJourney({ bookingID }: { bookingID: string }) {
  const [ready, setReady] = useState<Readiness | null>(null);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const [joinLink, setJoinLink] = useState("");
  const [verifiedCompletion, setVerifiedCompletion] = useState(false);
  const pendingFeedback = useRef<{ key: string; fingerprint: string } | null>(null);
  const pendingJoinKey = useRef<string | null>(null);
  const [progress, setProgress] = useState("0");
  const [satisfaction, setSatisfaction] = useState("5");
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [outcomeStatus, setOutcomeStatus] = useState("");
  useEffect(() => {
    let active = true;
    setReady(null); setOutcome(null); setStatus(""); setOutcomeStatus(""); setVerifiedCompletion(false);
    setJoinLink("");
    pendingFeedback.current = null;
    pendingJoinKey.current = null;
    // The specialist may confirm after the client. Reconcile both readiness
    // acknowledgments from PostgreSQL while the booking is open; no reload or
    // browser-local presence inference is required.
    let initialRead = true;
    async function refreshReadiness() {
      try {
        const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/ready`, {
          credentials: "include", cache: "no-store",
        });
        if (!response.ok) throw new Error("UNAVAILABLE");
        const payload: unknown = await response.json();
        if (active && validReady(payload, bookingID)) setReady(payload);
      } catch {
        if (active && initialRead) setStatus("Готовность пока нельзя проверить.");
      } finally { initialRead = false; }
    }
    void refreshReadiness();
    const readinessRefresh = setInterval(() => { void refreshReadiness(); }, 12_000);
    async function refreshVerifiedState() {
      try {
        const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/result`, {
          credentials: "include", cache: "no-store",
        });
        if (!response.ok) return;
        const data: unknown = await response.json();
        if (!active || !data || typeof data !== "object") return;
        const result = data as { booking_id?: string; state?: string; completion_evidence_ref?: string };
        if (result.booking_id !== bookingID || result.state !== "COMPLETED" ||
            typeof result.completion_evidence_ref !== "string" || !result.completion_evidence_ref.trim()) return;
        setVerifiedCompletion(true);
      } catch { /* never infer completion from a failed provider read */ }
    }
    void refreshVerifiedState();
    const refresh = setInterval(() => { void refreshVerifiedState(); }, 20_000);
    return () => { active = false; clearInterval(refresh); clearInterval(readinessRefresh); };
  }, [bookingID]);
  useEffect(() => {
    if (!verifiedCompletion) return;
    let active = true;
    void fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/outcome`, { credentials: "include", cache: "no-store" })
      .then(async (r) => r.ok ? r.json() as Promise<Outcome> : null)
      .then((payload) => { if (active && payload?.booking_id === bookingID) setOutcome(payload); })
      .catch(() => {});
    return () => { active = false; };
  }, [bookingID, verifiedCompletion]);

  async function confirmReady() {
    setBusy(true); setStatus("");
    try {
      const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/ready`, {
        method: "POST", credentials: "include", cache: "no-store", headers: { "X-APGIC-Request": "consultation-v1" },
      });
      const payload: unknown = await response.json();
      if (!response.ok || !validReady(payload, bookingID)) throw new Error("CLOSED");
      setReady(payload);
      setStatus("Ваша готовность сохранена в APGIC.");
    } catch { setStatus("Не удалось подтвердить готовность. Проверьте, что запись подтверждена и до начала осталось не более 15 минут."); }
    finally { setBusy(false); }
  }

  async function startJoin() {
    setBusy(true); setJoinLink(""); setStatus("");
    try {
      // Keep the same issuer idempotency key after network uncertainty;
      // reissuing with a fresh key could create another provider grant.
      const key = pendingJoinKey.current ?? `consultation-join:${crypto.randomUUID()}`;
      pendingJoinKey.current = key;
      const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/join`, {
        method: "POST", credentials: "include", cache: "no-store",
        headers: { "X-APGIC-Request": "consultation-v1", "Idempotency-Key": key },
      });
      const data: unknown = await response.json();
      const candidate = data as Partial<ConsultationJoinGrant>;
      if (!response.ok || candidate.booking_id !== bookingID ||
          candidate.credential_scope !== "ROOM_JOIN" ||
          typeof candidate.join_uri !== "string" || !candidate.join_uri.startsWith("https://")) {
        throw new Error("NO_VALID_GRANT");
      }
      const validated = new URL(candidate.join_uri);
      if (validated.protocol !== "https:" || validated.username || validated.password || !validated.hostname) {
        throw new Error("UNSAFE_JOIN_URI");
      }
      setJoinLink(candidate.join_uri);
      pendingJoinKey.current = null;
      setStatus("Доступ к комнате выдан внешним сервисом связи.");
    } catch { setStatus("Вход пока не подтверждён. Проверьте время записи и подключение сервиса связи. Повторная оплата не нужна."); }
    finally { setBusy(false); }
  }

  async function reportOutcome() {
    setBusy(true); setOutcomeStatus("");
    try {
      const fingerprint = `${bookingID}:${progress}:${satisfaction}`;
      if (pendingFeedback.current?.fingerprint !== fingerprint) {
        pendingFeedback.current = { fingerprint, key: `client-feedback:${crypto.randomUUID()}` };
      }
      const idempotencyKey = pendingFeedback.current.key;
      const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/outcome`, {
        method: "POST", credentials: "include", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-APGIC-Request": "consultation-v1" },
        body: JSON.stringify({ progress: Number(progress), match_satisfaction: Number(satisfaction), idempotency_key: idempotencyKey }),
      });
      const payload: unknown = await response.json();
      const data = payload as Partial<Outcome>;
      if (!response.ok || data?.booking_id !== bookingID || typeof data.progress !== "number" ||
          typeof data.match_satisfaction !== "number") throw new Error("NOT_SAVED");
      setOutcome(data as Outcome);
      pendingFeedback.current = null;
      setOutcomeStatus("Ваш отклик сохранён.");
    } catch { setOutcomeStatus("Не удалось сохранить отклик. Завершение консультации должно быть подтверждено внешним провайдером."); }
    finally { setBusy(false); }
  }

  return (
    <section className="journey-stage" aria-label="Готовность участников и личный отклик">
      <h3>Подготовка к консультации</h3>
      <p>Отметка «Я готов» не означает подключения к комнате или проведения консультации. Вход и завершение подтверждает внешний сервис связи.</p>
      {ready ? (
        <div aria-live="polite">
          <p>Клиент: {ready.client_ready_at ? "готов" : "готовность не подтверждена"}.
            Специалист: {ready.specialist_ready_at ? "готов" : "готовность не подтверждена"}.</p>
          {!ready.client_ready_at || !ready.specialist_ready_at ?
            <p role="status">Ожидаем отметки готовности обеих сторон. Состояние обновляется автоматически.</p> : null}
          <p>Окно готовности: {new Date(ready.opens_at).toLocaleString("ru-RU")} – {new Date(ready.closes_at).toLocaleString("ru-RU")}.</p>
          {ready.consultation_state === "TECHNICAL_FAILURE" ?
            <p role="alert">Медиасервис подтвердил технический сбой. Повторная выдача токена заблокирована. Возврат или перенос оформляется через владельца бронирования.</p> : null}
          {ready.consultation_state === "COMPLETED" ?
            <p role="status">Медиасервис подтвердил завершение консультации.</p> : null}
          {ready.booking_state === "CONFIRMED" &&
            ready.consultation_state !== "COMPLETED" &&
            ready.consultation_state !== "TECHNICAL_FAILURE" ? (
            <>
              <button type="button" disabled={busy} onClick={() => void confirmReady()}>Я готов</button>
              {((ready.role === "CLIENT" && ready.client_ready_at) ||
                (ready.role === "SPECIALIST" && ready.specialist_ready_at)) &&
                ready.client_ready_at && ready.specialist_ready_at ? (
                <button type="button" disabled={busy} onClick={() => void startJoin()}>Получить вход в консультацию</button>
              ) : null}
              {joinLink ? <p><a href={joinLink} target="_blank" rel="noopener noreferrer">Открыть защищённую комнату</a></p> : null}
            </>
          ) : null}
        </div>
      ) : null}
      {status ? <p role="status">{status}</p> : null}
      {verifiedCompletion ? (
        <div>
          <h3>Мой отклик после консультации</h3>
          <p>Это ваша субъективная оценка, не медицинское подтверждение результата. Аудио и текст беседы не собираются.</p>
          {outcome ? (
            <p role="status">Последняя оценка: изменение {outcome.progress}; удовлетворённость подбором {outcome.match_satisfaction} из 5.</p>
          ) : null}
          <label>Изменение состояния
            <select value={progress} onChange={(e) => setProgress(e.target.value)}>
              <option value="-2">Значительно хуже</option><option value="-1">Хуже</option>
              <option value="0">Без изменений</option><option value="1">Лучше</option><option value="2">Значительно лучше</option>
            </select>
          </label>
          <label>Насколько подошёл специалист
            <select value={satisfaction} onChange={(e) => setSatisfaction(e.target.value)}>
              {[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n} из 5</option>)}
            </select>
          </label>
          <button type="button" disabled={busy} onClick={() => void reportOutcome()}>Сохранить мой отклик</button>
          {outcomeStatus ? <p role="status">{outcomeStatus}</p> : null}
        </div>
      ) : null}
    </section>
  );
}
