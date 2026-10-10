"use client";

type SavedBooking = { id: string; booking_id: string; booking_state: string; expires_at: string };

type Props = {
  restoration: "CHECKING" | "READY" | "DENIED" | "UNAVAILABLE";
  booking: SavedBooking | null;
  onRetry: () => void;
};

export default function ResumeBooking({ restoration, booking, onRetry }: Props) {
  if (restoration === "DENIED" || (restoration === "READY" && !booking)) return null;

  return (
    <section className="resume-booking" aria-label="Продолжить предыдущую запись">
      <div>
        <p className="section-kicker">Предыдущая запись</p>
        <h2>{restoration === "UNAVAILABLE" ? "Проверить запись" : "Продолжить запись"}</h2>
        {restoration === "CHECKING" ? <p role="status">Проверяем сохранённую запись на сервере…</p> : null}
        {restoration === "UNAVAILABLE" ? (
          <p role="alert">Сервер не подтвердил состояние записи. Повторите проверку, прежде чем продолжать.</p>
        ) : null}
        {restoration === "READY" && booking ? (
          <p role="status">
            {booking.booking_state === "CONFIRMED" ? "Запись подтверждена сервером." :
              booking.booking_state === "HELD" || booking.booking_state === "PENDING_PAYMENT" ?
                "Время удерживалось временно. Это не подтверждённая запись — проверьте актуальный статус." :
                `Сервер сообщает о состоянии ${booking.booking_state}. Проверьте подробности перед дальнейшими действиями.`}
          </p>
        ) : null}
      </div>
      {restoration === "UNAVAILABLE" ? (
        <button type="button" className="button-link primary-action" onClick={onRetry}>Повторить проверку</button>
      ) : restoration === "READY" && booking ? (
        <a className="button-link primary-action" href="#previous-booking">Посмотреть мою запись</a>
      ) : null}
    </section>
  );
}
