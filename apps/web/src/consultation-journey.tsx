"use client";

import { useEffect, useRef, useState } from "react";

type Readiness = {
  booking_id: string; role: "CLIENT" | "SPECIALIST"; booking_state: string;
  opens_at: string; closes_at: string;
  client_ready_at?: string; specialist_ready_at?: string;
};
type Outcome = {
  booking_id: string; progress: number; match_satisfaction: number;
  reported_at: string;
};

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
  const [verifiedCompletion, setVerifiedCompletion] = useState(false);
  const pendingFeedback = useRef<{ key: string; fingerprint: string } | null>(null);
  const [progress, setProgress] = useState("0");
  const [satisfaction, setSatisfaction] = useState("5");
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [outcomeStatus, setOutcomeStatus] = useState("");
  useEffect(() => {
    let active = true;
    setReady(null); setOutcome(null); setStatus(""); setOutcomeStatus(""); setVerifiedCompletion(false);
    pendingFeedback.current = null;
    void fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/ready`, { credentials: "include", cache: "no-store" })
      .then(async (r) => {
        if (!r.ok) throw new Error("UNAVAILABLE");
        return r.json() as Promise<unknown>;
      }).then((payload) => { if (active && validReady(payload, bookingID)) setReady(payload); })
      .catch(() => { if (active) setStatus("Готовность пока нельзя проверить."); });
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
    return () => { active = false; clearInterval(refresh); };
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
          <p>Окно готовности: {new Date(ready.opens_at).toLocaleString("ru-RU")} – {new Date(ready.closes_at).toLocaleString("ru-RU")}.</p>
          {ready.booking_state === "CONFIRMED" ? (
            <button type="button" disabled={busy} onClick={() => void confirmReady()}>Я готов</button>
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
