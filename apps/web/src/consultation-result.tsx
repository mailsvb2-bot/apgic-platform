"use client";

import { useEffect, useRef, useState } from "react";
import type { ConsultationResult } from "../../../packages/contracts/src/generated/apgic-v1";

// Read-only projection of the canonical PostgreSQL consultation. This component
// never creates provider facts, payment capture, or a local completion state.
const KNOWN_STATES = new Set<ConsultationResult["state"]>([
  "SCHEDULED", "READY", "IN_PROGRESS", "RECOVERING", "TECHNICAL_FAILURE", "COMPLETED",
]);

const STATE_LABELS: Record<ConsultationResult["state"], string> = {
  SCHEDULED: "Запланирована",
  READY: "Участники готовы",
  IN_PROGRESS: "Идёт",
  RECOVERING: "Восстанавливается после сбоя",
  TECHNICAL_FAILURE: "Технический сбой",
  COMPLETED: "Завершена",
};

function isResult(value: unknown, bookingID: string): value is ConsultationResult {
  if (!value || typeof value !== "object") return false;
  const data = value as Partial<ConsultationResult>;
  return (
    data.booking_id === bookingID &&
    typeof data.state === "string" &&
    KNOWN_STATES.has(data.state as ConsultationResult["state"]) &&
    typeof data.provider_instance_id === "string" &&
    data.provider_instance_id.trim().length > 0 &&
    (data.completion_evidence_ref === undefined || typeof data.completion_evidence_ref === "string") &&
    (data.state !== "COMPLETED" || (
      typeof data.completion_evidence_ref === "string" &&
      data.completion_evidence_ref.trim().length > 0
    ))
  );
}

export default function ConsultationResultView({ bookingID }: { bookingID: string }) {
  const [result, setResult] = useState<ConsultationResult | null>(null);
  const [state, setState] = useState<"IDLE" | "LOADING" | "READY" | "NOT_FOUND" | "DENIED" | "ERROR">("IDLE");
  const [rating, setRating] = useState(0);
  const [feedback, setFeedback] = useState<"IDLE" | "SENDING" | "PENDING" | "NOT_ELIGIBLE" | "ERROR">("IDLE");
  // A stale asynchronous read must never show the outcome of a previous booking.
  const requestEpoch = useRef(0);
  useEffect(() => {
    requestEpoch.current += 1;
    setResult(null);
    setState("IDLE");
    setRating(0);
    setFeedback("IDLE");
    return () => { requestEpoch.current += 1; };
  }, [bookingID]);

  async function readResult() {
    const epoch = ++requestEpoch.current;
    setState("LOADING");
    setResult(null);
    try {
      const response = await fetch(`/v1/consultations/${encodeURIComponent(bookingID)}/result`, {
        credentials: "include",
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      if (epoch !== requestEpoch.current) return;
      if (response.status === 401 || response.status === 403) {
        setState("DENIED");
        return;
      }
      if (response.status === 404) {
        setState("NOT_FOUND");
        return;
      }
      if (!response.ok) {
        setState("ERROR");
        return;
      }
      const payload: unknown = await response.json();
      if (epoch !== requestEpoch.current) return;
      if (!isResult(payload, bookingID)) {
        setState("ERROR");
        return;
      }
      setResult(payload);
      setState("READY");
    } catch {
      if (epoch === requestEpoch.current) setState("ERROR");
    }
  }

  async function submitFeedback() {
    if (result?.state !== "COMPLETED" || !result.completion_evidence_ref ||
        !Number.isInteger(rating) || rating < 1 || rating > 5 || feedback !== "IDLE") return;
    // Client status is only an entry point. The API independently verifies
    // the signed client, canonical booking, completed state and uniqueness.
    const epoch = requestEpoch.current;
    setFeedback("SENDING");
    try {
      const response = await fetch("/v1/specialist-feedback", {
        method: "POST",
        credentials: "include",
        headers: { "content-type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ booking_id: bookingID, rating }),
      });
      if (epoch !== requestEpoch.current) return;
      if (response.status === 201) {
        const payload: unknown = await response.json();
        if (!payload || typeof payload !== "object" ||
            (payload as { state?: unknown }).state !== "PENDING") {
          setFeedback("ERROR");
          return;
        }
        setFeedback("PENDING");
      } else if (response.status === 409 || response.status === 401 || response.status === 403) {
        setFeedback("NOT_ELIGIBLE");
      } else {
        setFeedback("ERROR");
      }
    } catch {
      if (epoch === requestEpoch.current) setFeedback("ERROR");
    }
  }

  return (
    <section className="journey-stage" aria-label="Результат консультации">
      <h3>Результат консультации</h3>
      <p>Проверяется по сохранённым фактам провайдера. Статус не зависит от данных браузера.</p>
      <button type="button" onClick={() => void readResult()} disabled={state === "LOADING"}>
        {state === "LOADING" ? "Проверяем результат…" : "Проверить результат консультации"}
      </button>
      {state === "NOT_FOUND" ? <p role="status">Результат ещё не зарегистрирован. Проверьте позже.</p> : null}
      {state === "DENIED" ? <p role="alert">Нет подтверждённого доступа к результату. Повторно войдите в свою сессию.</p> : null}
      {state === "ERROR" ? <p role="alert">Не удалось подтвердить результат на сервере. Повторите проверку.</p> : null}
      {state === "READY" && result ? (
        <div role="status">
          <p>Состояние консультации: {STATE_LABELS[result.state]}.</p>
          {result.state === "COMPLETED" ? (
            <>
              <p>Подтверждение провайдера: {result.completion_evidence_ref}.</p>
              <p>Консультация завершена по подтверждённому событию провайдера.</p>
              <div aria-label="Оценка завершённой консультации">
                <label htmlFor="consultation-feedback-rating">Оцените проведённую консультацию</label>
                <select id="consultation-feedback-rating" value={rating}
                  disabled={feedback !== "IDLE"} onChange={(event) => setRating(Number(event.target.value))}>
                  <option value={0}>Выберите оценку</option>
                  {[1,2,3,4,5].map(value => <option value={value} key={value}>{value} из 5</option>)}
                </select>
                <button type="button" onClick={() => void submitFeedback()}
                  disabled={rating === 0 || feedback !== "IDLE"}>
                  {feedback === "SENDING" ? "Отправляем…" : "Отправить подтверждённую оценку"}
                </button>
                {feedback === "PENDING" ?
                  <p>Оценка зарегистрирована. Она появится в рейтинге только после независимой проверки.</p> : null}
                {feedback === "NOT_ELIGIBLE" ?
                  <p role="alert">Оценка недоступна: запись ещё не завершена, не принадлежит вам или уже оценена.</p> : null}
                {feedback === "ERROR" ?
                  <p role="alert">Не удалось подтвердить сохранение оценки. Проверьте результат перед повторной отправкой.</p> : null}
              </div>
            </>
          ) : (
            <p>Завершение консультации пока не подтверждено.</p>
          )}
        </div>
      ) : null}
    </section>
  );
}
