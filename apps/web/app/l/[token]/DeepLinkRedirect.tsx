"use client";

import { useEffect, useState } from "react";
import type { DeepLinkResolution } from "../../../../../packages/contracts/src/generated/apgic-v1";

function safeCanonicalFallback(value: DeepLinkResolution): string | null {
  if (
    value.decision !== "ALLOW" ||
    !value.canonical_path ||
    !value.canonical_web_fallback
  ) {
    return null;
  }
  try {
    const fallback = new URL(value.canonical_web_fallback);
    if (
      fallback.protocol !== "https:" ||
      fallback.hostname.toLowerCase() !== "apgic.ru" ||
      (fallback.port !== "" && fallback.port !== "443") ||
      fallback.username !== "" ||
      fallback.password !== "" ||
      fallback.search !== "" ||
      fallback.hash !== "" ||
      fallback.pathname !== value.canonical_path
    ) {
      return null;
    }
    return fallback.pathname;
  } catch {
    return null;
  }
}

export default function DeepLinkRedirect({ token }: { token: string }) {
  const [status, setStatus] = useState("Проверяем безопасную ссылку…");

  useEffect(() => {
    let active = true;
    void fetch(`/v1/mobile/deep-links/resolve?token=${encodeURIComponent(token)}`, {
      credentials: "include",
      headers: { Accept: "application/json" },
    })
      .then(async (response) => {
        if (!response.ok) {
          return null;
        }
        return (await response.json()) as DeepLinkResolution;
      })
      .then((resolution) => {
        if (!active || !resolution) {
          return;
        }
        const target = safeCanonicalFallback(resolution);
        if (!target) {
          setStatus("Ссылка недействительна или доступ к ресурсу не подтверждён.");
          return;
        }
        const next = new URL(target, window.location.origin);
        next.searchParams.set("link", token);
        window.location.replace(next.pathname + next.search);
      })
      .catch(() => {
        if (active) {
          setStatus("Не удалось безопасно открыть ссылку.");
        }
      });
    return () => {
      active = false;
    };
  }, [token]);

  return (
    <main>
      <h1>APGIC</h1>
      <p>{status}</p>
    </main>
  );
}
