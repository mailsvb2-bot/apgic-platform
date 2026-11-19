"use client";

import { useCallback, useEffect, useState } from "react";
import { growthURL } from "../../../src/growth-client";

type Offer = {
  id: string;
  topic_id: string;
  state: string;
  price_minor: number;
  currency: string;
};
type Report = {
  link_id: string;
  offer_id: string;
  source: string;
  state: string;
  touches: number;
  help_intents: number;
  confirmed_intents: number;
  attributed_holds: number;
  confirmed_bookings: number;
  provider_paid_orders: number;
};
type Link = {
  id: string;
  offer_id: string;
  topic_id: string;
  source: string;
  state: "ACTIVE" | "REVOKED" | "EXPIRED" | "UNAVAILABLE";
  expires_at: string;
  path?: string;
};
type Problem = { message_safe?: string; code?: string };
async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, credentials: "include", cache: "no-store" });
  const value = (await response.json()) as T & Problem;
  if (!response.ok) throw Error(value.message_safe || value.code || "Сервис временно недоступен.");
  return value;
}
function clock(value: string): string {
  return new Intl.DateTimeFormat("ru-RU", {
    day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit",
  }).format(new Date(value));
}
function status(value: Link["state"]): string {
  switch (value) {
    case "ACTIVE": return "Действует";
    case "EXPIRED": return "Истекла";
    case "REVOKED": return "Отозвана";
    case "UNAVAILABLE": return "Предложение больше недоступно";
    default: return "Недоступна";
  }
}
export function GrowthWorkspace() {
  const [offers, setOffers] = useState<Offer[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [reports, setReports] = useState<Report[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [growthUnavailable, setGrowthUnavailable] = useState(false);

  // Read each canonical owner independently. Missing Growth configuration must
  // not erase the already available specialist offers or silently invent links.
  const load = useCallback(async () => {
    const [supply, history, metrics] = await Promise.allSettled([
      api<{ offers: Offer[] }>("/v1/specialist/offers"),
      api<{ links: Link[] }>("/v1/specialist/growth-links"),
      api<{ items: Report[] }>("/v1/specialist/growth-report"),
    ]);
    const problems: string[] = [];
    if (supply.status === "fulfilled") {
      setOffers((supply.value.offers || []).filter(o => o.state === "ACTIVE"));
    } else {
      setOffers([]);
      problems.push(supply.reason instanceof Error ? supply.reason.message : "Не удалось загрузить предложения.");
    }
    if (history.status === "fulfilled") {
      setLinks(history.value.links || []);
      setGrowthUnavailable(false);
    } else {
      setLinks([]);
      const message = history.reason instanceof Error ? history.reason.message : "Не удалось загрузить ссылки.";
      if (/GROWTH_NOT_CONFIGURED|не настроено/.test(message)) setGrowthUnavailable(true);
      else {setGrowthUnavailable(false);problems.push(message)}
    }
    if (metrics.status === "fulfilled") setReports(metrics.value.items || []);
    else {
      setReports([]);
      const message = metrics.reason instanceof Error ? metrics.reason.message : "Не удалось загрузить отчёт.";
      if (!/GROWTH_NOT_CONFIGURED|не настроено/.test(message)) problems.push(message);
    }
    setError([...new Set(problems)].join(" "));
    return history.status === "fulfilled" ? (history.value.links || []) : null;
  }, []);

  useEffect(() => {
    let active = true;
    void load().catch(error => {
      if (active) setError(error instanceof Error ? error.message : "Не удалось прочитать кабинет.");
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => {active = false};
  }, [load]);

  const issue = async (offerID: string) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const confirmed = await api<Link>("/v1/specialist/growth-links", {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify({ offer_id: offerID }),
      });
      const readbackLinks = await load();
      const readbackLink = readbackLinks?.find(link => link.id === confirmed.id);
      if (readbackLink?.state === "ACTIVE" && readbackLink.path) {
        setNotice("Действующая ссылка готова. Повторное нажатие не создаёт новый источник.");
      } else if (readbackLink) {
        // A more recent owner-scoped readback wins over the preceding POST.
        // Never resurrect an already revoked, expired or unavailable link.
        setNotice("Статус ссылки изменился при повторной проверке. Актуальное состояние показано ниже; старая ссылка не выдаётся.");
      } else if (confirmed.state === "ACTIVE" && confirmed.path) {
        // The canonical POST already returned an owner-scoped signed link.
        // A failed GET must not erase this confirmed result or advertise
        // an invisible link. No locally synthesized tokens or paths.
        setLinks(previous => [confirmed, ...previous.filter(link => link.id !== confirmed.id)]);
        setNotice(readbackLinks === null
          ? "Ссылка выдана сервером и доступна ниже. Повторная проверка списка временно недоступна; не создавайте новую ссылку."
          : "Ссылка выдана сервером и доступна ниже, но ещё отсутствует в повторно прочитанном списке. Повторно создавать ссылку не нужно.");
      } else {
        setNotice("Сервер принял запрос, но не подтвердил доступную ссылку. Повторите проверку позже.");
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Невозможно создать ссылку.");
    } finally {
      setBusy(false);
    }
  };

  const share = async (path: string) => {
    setError("");
    const address = growthURL(path, window.location.origin);
    if (typeof navigator.share !== "function") {
      await copy(path);
      return;
    }
    try {
      await navigator.share({
        title: "Специалист в APGIC",
        text: "Выберите специалиста и доступное время в APGIC.",
        url: address,
      });
      setNotice("Вы выбрали способ поделиться предложением.");
    } catch (cause) {
      if (cause instanceof Error && cause.name === "AbortError") return;
      setError("Не удалось открыть меню отправки. Вы можете скопировать ссылку.");
    }
  };

  const revoke = async (id: string) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await api<{ state: Link["state"] }>(
        "/v1/specialist/growth-links/" + encodeURIComponent(id) + "/revoke",
        { method: "POST" },
      );
      if (result.state !== "REVOKED") {
        throw new Error("Сервер не подтвердил отзыв ссылки. Проверьте её состояние.");
      }
      // Invalidate the displayed signed path immediately after the canonical
      // owner confirms revocation; a lagging/failed GET must never resurrect it.
      const revoked = links.map(link => link.id === id
        ? { ...link, state: "REVOKED" as const, path: undefined } : link);
      setLinks(revoked);
      const checked = await load();
      if (checked === null) {
        setLinks(revoked);
        setNotice("Сервер подтвердил отзыв ссылки. Повторная проверка списка недоступна; отозванная ссылка не может быть распространена.");
      } else if (checked.some(link => link.id === id && link.state === "ACTIVE")) {
        setLinks(checked.map(link => link.id === id
          ? { ...link, state: "REVOKED" as const, path: undefined } : link));
        setNotice("Сервер подтвердил отзыв ссылки, но повторное чтение показывает прежнее состояние. Старая ссылка скрыта; проверьте список позже.");
      } else {
        setNotice("Ссылка отозвана. Ранее подтверждённые результаты сохранены.");
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось отозвать ссылку.");
    } finally {
      setBusy(false);
    }
  };

  const copy = async (path: string) => {
    setError("");
    try {
      await navigator.clipboard.writeText(growthURL(path, window.location.origin));
      setNotice("Ссылка скопирована.");
    } catch {
      setError("Не удалось скопировать автоматически. Выделите адрес ссылки вручную.");
    }
  };

  return <div>
    {error && <p role="alert">{error}</p>}
    {growthUnavailable && <p role="status">Привлечение клиентов по ссылкам пока не настроено. Ваши действующие предложения доступны ниже, но выдача новых ссылок временно отключена.</p>}
    {notice && <p role="status">{notice}</p>}
    {loading && <p role="status">Загружаем проверенные предложения и историю ссылок…</p>}
    {!loading && offers.length === 0 &&
      <p>Действующих предложений пока нет. Для распространения требуется опубликованный продукт и подтверждённая квалификация.</p>}
    <div className="how-grid">
      {offers.map((offer) => <article key={offer.id}>
        <h2>{offer.topic_id}</h2>
        <p>Цена: {new Intl.NumberFormat("ru-RU").format(offer.price_minor / 100)} {offer.currency}</p>
        <button type="button" disabled={busy||growthUnavailable} onClick={() => void issue(offer.id)}>
          Получить ссылку для предложения
        </button>
      </article>)}
    </div>
    <section aria-label="Мои ссылки распространения">
      <h2>Ссылки и история</h2>
      <p>Ссылки хранятся на сервере. Их можно повторно открыть после обновления страницы, пока они действуют. Исторические результаты не удаляются при отзыве.</p>
      {!loading && links.length === 0 && <p>Вы ещё не создавали ссылок.</p>}
      {links.map((link) => <article key={link.id}>
        <h3>{link.topic_id}</h3>
        <p>Статус: {status(link.state)} · до {clock(link.expires_at)}</p>
        {link.state === "ACTIVE" && link.path && <>
          <p><a href={link.path} rel="noreferrer">{growthURL(link.path, "https://apgic.ru")}</a></p>
          <button type="button" disabled={busy} onClick={() => void copy(link.path!)}>Скопировать ссылку</button>{" "}
          <button type="button" disabled={busy} onClick={() => void share(link.path!)}>Поделиться</button>{" "}
          <button type="button" disabled={busy} onClick={() => void revoke(link.id)}>Отозвать</button>
        </>}
      </article>)}
    </section>
    <section aria-label="Результаты привлечения">
      <h2>Результаты</h2>
      <p>Учитываем действительные переходы, клиентские запросы и бронирования. Платежи считаются только по внешним подтверждениям из ledger. Рекламные расходы, прибыль и CAC пока не рассчитываются.</p>
      {reports.map((item) => <article key={item.link_id}>
        <p>Предложение: {item.offer_id} · {item.source} · {status(item.state as Link["state"])}</p>
        <p>Переходов: {item.touches} · Запросов: {item.help_intents} · Подтверждено: {item.confirmed_intents}</p>
        <p>Броней: {item.attributed_holds} · Подтверждённых записей: {item.confirmed_bookings} · Оплаченных заказов: {item.provider_paid_orders}</p>
      </article>)}
    </section>
  </div>;
}
