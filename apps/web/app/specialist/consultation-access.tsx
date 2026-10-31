"use client";

import { FormEvent, useState } from "react";

const bookingPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

type Readiness = {
  booking_id: string;
  role: "CLIENT" | "SPECIALIST";
  booking_state: string;
  opens_at: string;
  closes_at: string;
};

function verifiedSpecialistReadiness(value: unknown, id: string): value is Readiness {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<Readiness>;
  return item.booking_id === id && item.role === "SPECIALIST" &&
    (item.booking_state === "CONFIRMED" || item.booking_state === "COMPLETED") &&
    typeof item.opens_at === "string" && Number.isFinite(Date.parse(item.opens_at)) &&
    typeof item.closes_at === "string" && Number.isFinite(Date.parse(item.closes_at));
}

/** Resume an existing consultation via canonical authorization, never a local list of bookings. */
export function SpecialistConsultationAccess() {
  const [bookingID, setBookingID] = useState("");
  const [verified, setVerified] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  async function check(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const id = bookingID.trim().toLowerCase();
    setVerified("");
    if (!bookingPattern.test(id)) {
      setNotice("Введите корректный идентификатор записи.");
      return;
    }
    setBusy(true);
    setNotice("Проверяем вашу роль и запись на сервере…");
    try {
      const response = await fetch(`/v1/consultations/${encodeURIComponent(id)}/ready`, {
        credentials: "include", cache: "no-store", headers: { Accept: "application/json" },
      });
      if (response.status === 401 || response.status === 403) {
        setNotice("Для доступа к консультации войдите в аккаунт специалиста.");
        return;
      }
      if (response.status === 404) {
        setNotice("Запись не найдена или доступ к ней отсутствует.");
        return;
      }
      if (!response.ok) {
        setNotice("Не удалось проверить запись. Повторите попытку.");
        return;
      }
      const data: unknown = await response.json();
      if (!verifiedSpecialistReadiness(data, id)) {
        setNotice("Сервер не подтвердил доступ специалиста к этой записи.");
        return;
      }
      setVerified(id);
      setNotice("Доступ специалиста подтверждён сервером. Можно открыть рабочее место.");
    } catch {
      setNotice("Сервис временно недоступен. Доступ не подтверждён.");
    } finally {
      setBusy(false);
    }
  }

  return <section aria-labelledby="specialist-consultation-title" className="start-section">
    <div className="start-heading">
      <p className="section-kicker">R2 → R3 · Мои консультации</p>
      <h2 id="specialist-consultation-title">Вернуться к своей консультации</h2>
      <p>Введите идентификатор существующей записи. APGIC проверит на сервере, что именно ваша профессиональная роль имеет доступ. Это не создаёт новую запись или оплату.</p>
    </div>
    <form onSubmit={check}>
      <label htmlFor="specialist-booking-id">Идентификатор записи</label>
      <input id="specialist-booking-id" value={bookingID} onChange={event => {
        setBookingID(event.target.value); setVerified(""); setNotice("");
      }} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" autoComplete="off" />
      <button type="submit" disabled={busy}>{busy ? "Проверяем…" : "Проверить доступ"}</button>
    </form>
    {notice && <p role="status">{notice}</p>}
    {verified && <p><a href={`/specialist/consultations/${verified}`}>Открыть рабочее место консультации</a></p>}
  </section>;
}
