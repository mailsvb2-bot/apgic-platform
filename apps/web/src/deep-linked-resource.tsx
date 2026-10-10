"use client";

import { useEffect, useState } from "react";
import type { DeepLinkResolution } from "../../../packages/contracts/src/generated/apgic-v1";

type ResourceKind = "SPECIALIST" | "BOOKING" | "NOTIFICATION";

type ConsultationResult = {
  booking_id: string;
  state: string;
  provider_instance_id: string;
  completion_evidence_ref?: string;
};

const titles: Record<ResourceKind, string> = {
  SPECIALIST: "Специалист",
  BOOKING: "Бронирование",
  NOTIFICATION: "Уведомление",
};

function validResolution(
  value: unknown,
  expectedPath: string,
): value is DeepLinkResolution {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<DeepLinkResolution>;
  if (
    candidate.decision !== "ALLOW" ||
    candidate.reason_code !== "DEEPLINK_ALLOWED" ||
    candidate.canonical_path !== expectedPath ||
    typeof candidate.canonical_web_fallback !== "string"
  ) {
    return false;
  }
  try {
    const fallback = new URL(candidate.canonical_web_fallback);
    return (
      fallback.protocol === "https:" &&
      fallback.hostname.toLowerCase() === "apgic.ru" &&
      (fallback.port === "" || fallback.port === "443") &&
      fallback.username === "" &&
      fallback.password === "" &&
      fallback.search === "" &&
      fallback.hash === "" &&
      fallback.pathname === expectedPath
    );
  } catch {
    return false;
  }
}

export default function DeepLinkedResource({
  kind,
  id,
}: {
  kind: ResourceKind;
  id: string;
}) {
  const expectedPath =
    kind === "SPECIALIST"
      ? `/specialists/${id}`
      : kind === "BOOKING"
        ? `/bookings/${id}`
        : `/notifications/${id}`;
  const [state, setState] = useState<"CHECKING" | "ALLOWED" | "DENIED" | "ERROR">(
    "CHECKING",
  );

  const [consultation, setConsultation] = useState<ConsultationResult | null>(null);
  const [resultState, setResultState] = useState<"IDLE" | "LOADING" | "NOT_FOUND" | "ERROR" | "READY">("IDLE");

  async function readConsultationResult() {
    setResultState("LOADING");
    setConsultation(null);
    try {
      const response = await fetch(`/v1/consultations/${encodeURIComponent(id)}/result`, {
        credentials: "include",
        cache: "no-store",
      });
      if (response.status === 404) {
        setResultState("NOT_FOUND");
        return;
      }
      if (!response.ok) {
        setResultState("ERROR");
        return;
      }
      const payload = await response.json() as Partial<ConsultationResult>;
      if (payload.booking_id !== id || typeof payload.state !== "string" || typeof payload.provider_instance_id !== "string") {
        setResultState("ERROR");
        return;
      }
      setConsultation(payload as ConsultationResult);
      setResultState("READY");
    } catch {
      setResultState("ERROR");
    }
  }

  useEffect(() => {
    let active = true;
    const storageKey = `apgic:deeplink:${expectedPath}`;
    const token = sessionStorage.getItem(storageKey);
    if (!token) {
      setState("DENIED");
      return () => {
        active = false;
      };
    }
    void fetch(
      `/v1/mobile/deep-links/resolve?token=${encodeURIComponent(token)}`,
      {
        credentials: "include",
        headers: { Accept: "application/json" },
        cache: "no-store",
      },
    )
      .then(async (response) => {
        if (!response.ok) {
          return null;
        }
        return response.json() as Promise<unknown>;
      })
      .then((resolution) => {
        if (!active) {
          return;
        }
        if (validResolution(resolution, expectedPath)) {
          setState("ALLOWED");
        } else {
          sessionStorage.removeItem(storageKey);
          setState("DENIED");
        }
      })
      .catch(() => {
        if (active) {
          setState("ERROR");
        }
      });
    return () => {
      active = false;
    };
  }, [expectedPath]);

  return (
    <main>
      <h1>{titles[kind]}</h1>
      {state === "CHECKING" ? <p>Проверяем доступ к ресурсу…</p> : null}
      {state === "ALLOWED" ? (
        <>
          <p role="status">Ресурс подтверждён сервером.</p>
          <p>Идентификатор: {id}</p>
          {kind === "BOOKING" ? (
            <section aria-label="Результат консультации">
              <h2>Результат консультации</h2>
              <button type="button" disabled={resultState === "LOADING"} onClick={() => void readConsultationResult()}>
                {resultState === "LOADING" ? "Проверяем результат…" : "Проверить результат консультации"}
              </button>
              {resultState === "NOT_FOUND" ? <p role="status">Подтверждённый результат консультации пока не найден.</p> : null}
              {resultState === "ERROR" ? <p role="alert">Не удалось подтвердить результат на сервере. Повторите попытку.</p> : null}
              {resultState === "READY" && consultation ? (
                <div role="status">
                  <p>Состояние: {consultation.state === "COMPLETED" ? "Завершена" : "Не завершена"}</p>
                  {consultation.state === "COMPLETED" && consultation.completion_evidence_ref ? <p>Доказательство завершения: {consultation.completion_evidence_ref}</p> : null}
                </div>
              ) : null}
            </section>
          ) : null}
        </>
      ) : null}
      {state === "DENIED" ? (
        <p role="alert">Ссылка недействительна или доступ к ресурсу не подтверждён.</p>
      ) : null}
      {state === "ERROR" ? (
        <p role="alert">Не удалось проверить доступ к ресурсу.</p>
      ) : null}
    </main>
  );
}
