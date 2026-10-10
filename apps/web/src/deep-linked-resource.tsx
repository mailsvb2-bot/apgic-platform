"use client";

import { useEffect, useState } from "react";
import ConsultationResultView from "./consultation-result";
import type { DeepLinkResolution } from "../../../packages/contracts/src/generated/apgic-v1";

type ResourceKind = "SPECIALIST" | "BOOKING" | "NOTIFICATION";

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
          {kind === "BOOKING" ? <ConsultationResultView key={id} bookingID={id} /> : null}
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
