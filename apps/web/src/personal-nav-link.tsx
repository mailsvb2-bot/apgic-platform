"use client";

import {useCallback, useEffect, useState} from "react";

/**
 * An account entry is shown only after the backend confirms a signed client
 * session. Merely having a stale browser flag, a client-supplied Identity,
 * or a rejected/failed response can never advertise account ownership.
 *
 * This preserves the anonymous acquisition page while making the personal
 * workspace discoverable as soon as the first server-side session is issued.
 */
export default function PersonalNavLink() {
  const [authorized, setAuthorized] = useState(false);
  const check = useCallback(async (signal: AbortSignal) => {
    try {
      const response = await fetch("/v1/client/personal", {
        method: "GET", credentials: "include", cache: "no-store", signal,
        headers: {Accept: "application/json"},
      });
      if (!signal.aborted) setAuthorized(response.status === 200);
    } catch {
      if (!signal.aborted) setAuthorized(false);
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    const refresh = () => void check(controller.signal);
    refresh();
    window.addEventListener("apgic:client-session-established", refresh);
    window.addEventListener("focus", refresh);
    return () => {
      controller.abort();
      window.removeEventListener("apgic:client-session-established", refresh);
      window.removeEventListener("focus", refresh);
    };
  }, [check]);
  return authorized ? <a href="/personal">Мой кабинет</a> : null;
}
