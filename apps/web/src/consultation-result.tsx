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
  // A stale asynchronous read must never show the outcome of a previous booking.
  const requestEpoch = useRef(0);
  useEffect(() => {
    requestEpoch.current += 1;
    setResult(null);
    setState("IDLE");
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
            <p>Подтверждение провайдера: {result.completion_evidence_ref}.</p>
          ) : (
            <p>Завершение консультации пока не подтверждено.</p>
          )}
        </div>
      ) : null}
    </section>
  );
}
