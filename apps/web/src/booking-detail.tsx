"use client";

import { useEffect, useState } from "react";
import ConsultationResultView from "./consultation-result";

import type { BookingStatus, OrderStatus } from "../../../packages/contracts/src/generated/apgic-v1";

type BookingState = BookingStatus["state"];

const labels: Record<BookingState, string> = {
  HELD: "Время временно удерживается",
  PENDING_PAYMENT: "Ожидается подтверждение оплаты внешним провайдером",
  CONFIRMED: "Запись подтверждена",
  CANCELLED: "Запись отменена",
  EXPIRED: "Срок удержания времени истёк",
  COMPLETED: "Запись завершена",
  NO_SHOW: "Неявка",
};

function validBooking(value: unknown, expectedID: string): value is BookingStatus {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<BookingStatus>;
  return (
    candidate.booking_id === expectedID &&
    typeof candidate.state === "string" &&
    Object.prototype.hasOwnProperty.call(labels, candidate.state) &&
    typeof candidate.hold_id === "string" && candidate.hold_id.trim().length > 0 &&
    typeof candidate.hold_expires_at === "string" &&
    Number.isFinite(Date.parse(candidate.hold_expires_at)) &&
    (candidate.starts_at === undefined || (
      typeof candidate.starts_at === "string" && Number.isFinite(Date.parse(candidate.starts_at))
    )) &&
    (candidate.ends_at === undefined || (
      typeof candidate.ends_at === "string" && Number.isFinite(Date.parse(candidate.ends_at))
    )) &&
    (!(candidate.state === "CONFIRMED" || candidate.state === "COMPLETED") ||
      (candidate.starts_at !== undefined && candidate.ends_at !== undefined))
  );
}

export default function BookingDetail({ bookingID }: { bookingID: string }) {
  const [retry, setRetry] = useState(0);
  const [view, setView] = useState<{ id: string; booking: BookingStatus } | null>(null);
  const [status, setStatus] = useState<"CHECKING" | "READY" | "DENIED" | "NOT_FOUND" | "UNAVAILABLE">("CHECKING");
  const [order, setOrder] = useState<OrderStatus | null>(null);
  const [orderStatus, setOrderStatus] = useState<"CHECKING" | "VERIFIED" | "NO_CHECKOUT" | "UNAVAILABLE">("CHECKING");

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setStatus("CHECKING");
    setView(null);
    setOrder(null);
    setOrderStatus("CHECKING");
    void (async () => {
      try {
        const response = await fetch(`/v1/bookings/${encodeURIComponent(bookingID)}`, {
          method: "GET",
          credentials: "include",
          cache: "no-store",
          headers: { Accept: "application/json" },
          signal: controller.signal,
        });
        if (!active) return;
        if (response.status === 401 || response.status === 403) {
          setStatus("DENIED");
          return;
        }
        if (response.status === 404) {
          setStatus("NOT_FOUND");
          return;
        }
        if (!response.ok) {
          setStatus("UNAVAILABLE");
          return;
        }
        const payload: unknown = await response.json();
        if (!active) return;
        if (!validBooking(payload, bookingID)) {
          setStatus("UNAVAILABLE");
          return;
        }
        const financial = await fetch(`/v1/bookings/${encodeURIComponent(bookingID)}/order`, {
          method: "GET",
          credentials: "include",
          cache: "no-store",
          headers: { Accept: "application/json" },
          signal: controller.signal,
        });
        if (!active) return;
        if (financial.status === 404) {
          setOrderStatus("NO_CHECKOUT");
        } else if (!financial.ok) {
          setOrderStatus("UNAVAILABLE");
        } else {
          const economic: unknown = await financial.json();
          if (!active) return;
          if (!economic || typeof economic !== "object") {
            setOrderStatus("UNAVAILABLE");
          } else {
            const record = economic as Partial<OrderStatus>;
            if (record.booking_id !== bookingID ||
                typeof record.order_id !== "string" || !record.order_id ||
                record.booking_state !== payload.state ||
                !["PENDING_PROVIDER", "CAPTURED"].includes(record.payment_state ?? "") ||
                typeof record.amount_minor !== "number" || !Number.isSafeInteger(record.amount_minor) || record.amount_minor <= 0 ||
                typeof record.currency !== "string" || !/^[A-Z]{3}$/.test(record.currency) ||
                typeof record.provider_id !== "string" || !record.provider_id ||
                typeof record.payment_method !== "string" || !record.payment_method ||
                (record.refund_state && record.payment_state !== "CAPTURED")) {
              setOrderStatus("UNAVAILABLE");
            } else {
              setOrder(record as OrderStatus);
              setOrderStatus("VERIFIED");
            }
          }
        }
        setView({ id: bookingID, booking: payload });
        setStatus("READY");
      } catch {
        if (active) setStatus("UNAVAILABLE");
      }
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [bookingID, retry]);

  const booking = view?.id === bookingID ? view.booking : null;
  return (
    <main>
      <h1>Моя запись</h1>
      <p>Актуальное состояние запрашивается у APGIC. Данные этой страницы не подтверждают оплату сами по себе.</p>
      {status === "CHECKING" ? <p role="status">Проверяем запись на сервере…</p> : null}
      {status === "DENIED" ? <p role="alert">Для просмотра своей записи необходимо войти в аккаунт.</p> : null}
      {status === "NOT_FOUND" ? <p role="alert">Запись не найдена или не принадлежит вашему аккаунту.</p> : null}
      {status === "UNAVAILABLE" ? <p role="alert">Не удалось проверить запись на сервере. Состояние не подтверждено.</p> : null}
      {status === "READY" && booking ? (
        <section aria-label="Подтверждённое состояние записи">
          <h2>{labels[booking.state]}</h2>
          <p>Идентификатор записи: {booking.booking_id}</p>
          {orderStatus === "CHECKING" ? <p role="status">Проверяем платёж у внешнего исполнителя…</p> : null}
          {orderStatus === "UNAVAILABLE" ? <p role="alert">Платёжные сведения не удалось проверить. Не считайте запись оплаченной только по её статусу.</p> : null}
          {orderStatus === "NO_CHECKOUT" ? <p role="status">Заказ на оплату ещё не создан либо недоступен. Подтверждения списания нет.</p> : null}
          {orderStatus === "VERIFIED" && order ? (
            <section aria-label="Подтверждённые сведения о платеже">
              <h3>Состояние оплаты</h3>
              <p>Заказ: {order.order_id}</p>
              <p>{order.payment_state === "CAPTURED" ? "Списание подтверждено провайдером." : "Подтверждение оплаты от провайдера пока не получено."}</p>
              {order.refund_state ? <p>Возврат через провайдера: {order.refund_state}.</p> : null}
            </section>
          ) : null}
          {booking.starts_at ? <p>Начало: {new Date(booking.starts_at).toLocaleString("ru-RU")}.</p> : null}
          {booking.ends_at ? <p>Окончание: {new Date(booking.ends_at).toLocaleString("ru-RU")}.</p> : null}
          {booking.state === "HELD" || booking.state === "PENDING_PAYMENT" ?
            <p>Это ещё не подтверждённая консультация. Не совершайте повторную оплату без проверки заказа.</p> : null}
          {booking.state === "CONFIRMED" || booking.state === "COMPLETED" ?
            <ConsultationResultView key={bookingID} bookingID={bookingID} /> : null}
        </section>
      ) : null}
      <button type="button" onClick={() => setRetry((value) => value + 1)}
        disabled={status === "CHECKING"}>Проверить актуальный статус</button>
      <p><a href="/">Вернуться на главную</a></p>
    </main>
  );
}
