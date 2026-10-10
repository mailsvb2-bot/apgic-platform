"use client";

import { useEffect, useState } from "react";

type SavedBooking = { id: string; booking_id: string; booking_state: string; expires_at: string };

export default function ResumeBooking() {
  const [state, setState] = useState<"NONE" | "CHECKING" | "FOUND" | "UNAVAILABLE">("NONE");
  const [booking, setBooking] = useState<SavedBooking | null>(null);

  useEffect(() => {
    const id = sessionStorage.getItem("apgic:current-hold-id");
    if (!id) return;
    let active = true;
    setState("CHECKING");
    void fetch(`/v1/slot-holds/${encodeURIComponent(id)}`, { cache: "no-store", credentials: "include" })
      .then(async (response) => {
        if ([401, 403, 404].includes(response.status)) {
          sessionStorage.removeItem("apgic:current-hold-id");
          if (active) setState("NONE");
          return;
        }
        if (!response.ok) throw new Error("Booking status unavailable");
        const data = await response.json() as Partial<SavedBooking>;
        if (data.id !== id || typeof data.booking_id !== "string" || typeof data.booking_state !== "string" || typeof data.expires_at !== "string") {
          throw new Error("Invalid booking response");
        }
        if (active) {
          setBooking(data as SavedBooking);
          setState("FOUND");
        }
      }).catch(() => { if (active) setState("UNAVAILABLE"); });
    return () => { active = false; };
  }, []);

  if (state === "NONE") return null;
  return (
    <section className="resume-booking" aria-label="Продолжить предыдущую запись">
      <div>
        <p className="section-kicker">Вы уже начали запись</p>
        <h2>Продолжить запись</h2>
        {state === "CHECKING" ? <p role="status">Проверяем сохранённую запись на сервере…</p> : null}
        {state === "UNAVAILABLE" ? <p role="alert">Не удалось проверить предыдущую запись. Откройте подбор, чтобы повторить проверку.</p> : null}
        {state === "FOUND" && booking ? (
          <p role="status">
            {booking.booking_state === "CONFIRMED" ? "Запись подтверждена сервером." :
              booking.booking_state === "HELD" ? "Время было временно удержано. Проверьте актуальность перед продолжением." :
              "Состояние записи изменилось. Посмотрите актуальные варианты."}
          </p>
        ) : null}
      </div>
      <a className="button-link primary-action" href="#start">Открыть мою запись</a>
    </section>
  );
}
