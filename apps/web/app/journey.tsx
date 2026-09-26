"use client";

import { FormEvent, useState } from "react";

type Intent = {
  id: string;
  client_identity_id: string;
  free_text: string;
  topics: string[];
  goals: string[];
  status: string;
  notice: string;
  diagnosis_asserted: boolean;
  catalog_mode: string;
};

type MatchCard = {
  specialist_id: string;
  display_name: string;
  profession: string;
  price_minor: number;
  currency: string;
  format: string;
  sponsored: boolean;
};

type Slot = {
  id: string;
  starts_at: string;
  ends_at: string;
  exclusive: boolean;
};

type Hold = {
  id: string;
  booking_id: string;
  state: string;
  booking_state: string;
  expires_at: string;
  reason_code: string;
};

type CheckoutOption = {
  method_code: string;
  amount_minor: number;
  currency: string;
  payment_recipient_id: string;
  execution_owner: string;
  apgic_accepts_funds: boolean;
};

type CheckoutInstruction = {
  order_id: string;
  booking_state: string;
  provider_id: string;
  method_code: string;
  amount_minor: number;
  currency: string;
  payment_recipient_id: string;
  execution_owner: string;
  apgic_accepts_funds: boolean;
  notice: string;
};

type PaymentEvidence = {
  booking_id: string;
  booking_state: string;
  ledger_entry_id: string;
  credit_account_ref: string;
  apgic_accepts_funds: boolean;
  idempotent: boolean;
  notice: string;
};

type Cancellation = {
  booking_state: string;
  refund_state: string;
  provider_id: string;
  execution_owner: string;
  original_ledger_id: string;
  reversal_ledger_id: string;
  apgic_accepts_funds: boolean;
  apgic_returns_funds: boolean;
  idempotent: boolean;
  notice: string;
};

const METHOD_LABELS: Record<string, string> = {
  BANK_CARD: "Карта через внешнего провайдера",
  SBP: "СБП через внешнего провайдера",
};

const TOPICS = [
  { id: "anxiety", label: "Тревога и волнение" },
  { id: "sleep", label: "Сон" },
  { id: "career", label: "Карьера и работа" },
  { id: "relationships", label: "Отношения" },
] as const;

const PROFESSIONS: Record<string, string> = {
  PSYCHOLOGIST: "Психолог",
  CAREER_COACH: "Карьерный консультант",
};

function clientUUID() {
  const value = new Uint8Array(16);
  globalThis.crypto.getRandomValues(value);
  value[6] = (value[6] & 0x0f) | 0x40;
  value[8] = (value[8] & 0x3f) | 0x80;
  const hex = Array.from(value, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join("-");
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "content-type": "application/json", "x-correlation-id": clientUUID() },
    body: JSON.stringify(body),
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.message_safe || "Не удалось выполнить запрос.");
  }
  return payload as T;
}

function money(minor: number, currency: string) {
  const amount = new Intl.NumberFormat("ru-RU").format(minor / 100);
  return currency === "RUB" ? `${amount} ₽` : `${amount} ${currency}`;
}

function when(value: string) {
  return new Intl.DateTimeFormat("ru-RU", {
    weekday: "short",
    day: "numeric",
    month: "long",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function Journey() {
  const [text, setText] = useState("");
  const [intent, setIntent] = useState<Intent | null>(null);
  const [topics, setTopics] = useState<string[]>([]);
  const [matches, setMatches] = useState<MatchCard[] | null>(null);
  const [search, setSearch] = useState<{ notice: string; stale: boolean; rebuilt: boolean; owns_qualification: boolean; entries: { specialist_id: string; display_name: string }[] } | null>(null);
  const [specialist, setSpecialist] = useState<MatchCard | null>(null);
  const [slots, setSlots] = useState<Slot[]>([]);
  const [hold, setHold] = useState<Hold | null>(null);
  const [options, setOptions] = useState<CheckoutOption[]>([]);
  const [instruction, setInstruction] = useState<CheckoutInstruction | null>(null);
  const [evidence, setEvidence] = useState<PaymentEvidence | null>(null);
  const [cancellation, setCancellation] = useState<Cancellation | null>(null);
  const [fulfillment, setFulfillment] = useState<{ notice: string; join: string } | null>(null);
  const [session, setSession] = useState<{
    state: string;
    notice: string;
    evidence_ref?: string;
    charged_again: boolean;
    raw_content_stored?: boolean;
    refund_path_opened?: boolean;
    apgic_returns_funds?: boolean;
  } | null>(null);
  const [providerEventID] = useState(() => clientUUID());
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [deletion, setDeletion] = useState<{
    state: string;
    deactivation: boolean;
    profile_erased: boolean;
    ledger_retained: boolean;
    apgic_deletes_ledger: boolean;
    notice: string;
    idempotent: boolean;
  } | null>(null);

  async function interpret(event: FormEvent) {
    event.preventDefault();
    setError("");
    setPending(true);
    setMatches(null);
    setSpecialist(null);
    setHold(null);
    try {
      const created = await postJSON<Intent>("/v1/help-intents", { free_text: text });
      setIntent(created);
      setTopics(created.topics);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось разобрать запрос.");
    } finally {
      setPending(false);
    }
  }

  function toggleTopic(id: string) {
    setTopics((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }

  async function confirm(event: FormEvent) {
    event.preventDefault();
    if (!intent) return;
    setError("");
    setPending(true);
    setHold(null);
    setSpecialist(null);
    try {
      const confirmed = await postJSON<Intent>(`/v1/help-intents/${intent.id}/confirm`, { topics });
      setIntent(confirmed);
      const listed = await fetch(`/v1/help-intents/${confirmed.id}/matches`).then(async (response) => {
        const payload = await response.json();
        if (!response.ok) throw new Error(payload.message_safe || "Подбор недоступен.");
        return payload as { matches: MatchCard[] };
      });
      setMatches(listed.matches);
      const topic = topics[0];
      if (topic) {
        const projected = await fetch(`/v1/search?topic=${encodeURIComponent(topic)}`);
        const projection = await projected.json();
        if (!projected.ok) throw new Error(projection.message_safe || "Поиск недоступен.");
        if (projection.owns_qualification) throw new Error("Поиск не должен владеть допуском.");
        setSearch(projection);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось подтвердить запрос.");
    } finally {
      setPending(false);
    }
  }

  async function staleSearch() {
    const topic = topics[0];
    if (!topic) return;
    setError("");
    setPending(true);
    try {
      const view = await postJSON<{ owns_qualification: boolean; stale: boolean; entries: { specialist_id: string; display_name: string }[]; notice: string; rebuilt: boolean }>("/v1/search/stale", {
        topic,
        specialist_id: "spec-lebedeva",
      });
      if (view.owns_qualification) throw new Error("Поиск не должен владеть допуском.");
      setSearch(view);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Индекс не сброшен.");
    } finally {
      setPending(false);
    }
  }

  async function rebuildSearch() {
    const topic = topics[0];
    if (!topic) return;
    setError("");
    setPending(true);
    try {
      const view = await postJSON<{ owns_qualification: boolean; stale: boolean; rebuilt: boolean; entries: { specialist_id: string; display_name: string }[]; notice: string }>("/v1/search/rebuild", { topic });
      if (view.owns_qualification || view.stale || !view.rebuilt) throw new Error("Проекция не восстановлена из каталога.");
      setSearch(view);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Поиск не восстановлен.");
    } finally {
      setPending(false);
    }
  }

  async function choose(card: MatchCard) {
    setError("");
    setPending(true);
    setHold(null);
    try {
      const response = await fetch(`/v1/specialists/${card.specialist_id}/slots`);
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.message_safe || "Слоты недоступны.");
      setSpecialist(card);
      setSlots(payload.slots);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось открыть слоты.");
    } finally {
      setPending(false);
    }
  }

  async function acquire(slot: Slot) {
    if (!intent) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<Hold>("/v1/slot-holds", {
        help_intent_id: intent.id,
        slot_id: slot.id,
        client_identity_id: intent.client_identity_id,
      });
      setHold(created);
      setInstruction(null);
      const listed = await fetch(`/v1/slot-holds/${created.id}/checkout-options?client_identity_id=${encodeURIComponent(intent.client_identity_id)}`);
      const payload = await listed.json();
      if (!listed.ok) throw new Error(payload.message_safe || "Способы оплаты недоступны.");
      setOptions(payload.options);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Слот не удержан.");
    } finally {
      setPending(false);
    }
  }

  async function chooseMethod(methodCode: string) {
    if (!intent || !hold) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<CheckoutInstruction>("/v1/checkout-instructions", {
        hold_id: hold.id,
        client_identity_id: intent.client_identity_id,
        method_code: methodCode,
      });
      if (created.apgic_accepts_funds || created.execution_owner !== "EXTERNAL_PROVIDER") {
        throw new Error("APGIC не принимает деньги.");
      }
      setInstruction(created);
      setEvidence(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Поручение не создано.");
    } finally {
      setPending(false);
    }
  }

  async function captureProviderEvent() {
    if (!instruction) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<PaymentEvidence>("/v1/provider-events", {
        provider_id: instruction.provider_id,
        provider_event_id: providerEventID,
        order_id: instruction.order_id,
        amount_minor: instruction.amount_minor,
        currency: instruction.currency,
        outcome: "CAPTURED",
      });
      if (created.apgic_accepts_funds) throw new Error("APGIC не принимает деньги.");
      setEvidence(created);
      if (intent) {
        const listed = await fetch(`/v1/bookings/${created.booking_id}/fulfillment?identity_id=${encodeURIComponent(intent.client_identity_id)}`);
        const payload = await listed.json();
        if (!listed.ok) throw new Error(payload.message_safe || "Допуск к консультации недоступен.");
        setFulfillment({
          notice: payload.notice?.requires_growth_opt_in ? "Нужно маркетинговое согласие." : "Служебное уведомление о брони отправлено без маркетингового согласия.",
          join: payload.join?.notice ?? "Вход в консультацию закрыт.",
        });
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Подтверждение провайдера не принято.");
    } finally {
      setPending(false);
    }
  }

  async function recordPresence() {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{ state: string; notice: string; charged_again: boolean }>("/v1/consultations/" + evidence.booking_id + "/presence", {});
      if (created.charged_again) throw new Error("Повторная оплата запрещена.");
      setSession({ state: created.state, notice: created.notice, charged_again: created.charged_again });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Факты сессии не записаны.");
    } finally {
      setPending(false);
    }
  }

  async function reportFailure(recoverable: boolean) {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{
        state: string;
        notice: string;
        charged_again: boolean;
        refund_path_opened: boolean;
        apgic_returns_funds: boolean;
        recovery_action?: string;
      }>("/v1/consultations/" + evidence.booking_id + "/failures", {
        kind: recoverable ? "PROVIDER_DISCONNECT" : "ROOM_FAILURE",
        evidence_ref: recoverable ? "provider-evidence/disconnect-1" : "provider-evidence/room-1",
        recoverable,
      });
      if (created.charged_again || created.apgic_returns_funds) throw new Error("APGIC не списывает и не возвращает деньги.");
      if (created.state === "COMPLETED") throw new Error("Сбой не завершает консультацию.");
      setSession(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Сбой связи не зафиксирован.");
    } finally {
      setPending(false);
    }
  }

  async function succeedRecovery() {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{ state: string; notice: string; charged_again: boolean; apgic_returns_funds: boolean }>(
        "/v1/consultations/" + evidence.booking_id + "/recovery",
        { evidence_ref: "provider-evidence/recovered-1" },
      );
      if (created.charged_again || created.apgic_returns_funds) throw new Error("APGIC не списывает и не возвращает деньги.");
      setSession(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Восстановление не зафиксировано.");
    } finally {
      setPending(false);
    }
  }

  async function exportToGrowth() {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{ raw_content_included: boolean }>("/v1/consultations/" + evidence.booking_id + "/growth-export", { purpose_consent: false });
      if (!created.raw_content_included) throw new Error("Сырая запись не должна была уйти в рост.");
      setError("Сырая запись не должна была уйти в рост.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Выгрузка отклонена.");
    } finally {
      setPending(false);
    }
  }

  async function completeWithoutEvidence() {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      await postJSON("/v1/consultations/" + evidence.booking_id + "/complete", { evidence_ref: "" });
      setError("Завершение без доказательства не должно было пройти.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Завершение отклонено.");
    } finally {
      setPending(false);
    }
  }

  async function completeWithEvidence() {
    if (!evidence) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{ state: string; notice: string; evidence_ref?: string; charged_again: boolean }>(
        "/v1/consultations/" + evidence.booking_id + "/complete",
        { evidence_ref: "provider-room-end" },
      );
      if (created.charged_again) throw new Error("Повторная оплата запрещена.");
      setSession(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Консультация не завершена.");
    } finally {
      setPending(false);
    }
  }

  async function cancelBooking() {
    if (!instruction) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<Cancellation>("/v1/cancellations", {
        order_id: instruction.order_id,
        reason_code: "CLIENT_CANCEL",
      });
      if (created.apgic_accepts_funds || created.apgic_returns_funds || created.execution_owner !== "EXTERNAL_PROVIDER") {
        throw new Error("Возврат исполняет только внешний провайдер.");
      }
      setCancellation(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Отмена не выполнена.");
    } finally {
      setPending(false);
    }
  }

  async function deleteAccount() {
    if (!intent) return;
    setError("");
    setPending(true);
    try {
      const created = await postJSON<{
        state: string;
        deactivation: boolean;
        profile_erased: boolean;
        ledger_retained: boolean;
        apgic_deletes_ledger: boolean;
        notice: string;
        idempotent: boolean;
      }>("/v1/account-deletions", { identity_id: intent.client_identity_id, source: "WEB" });
      if (created.deactivation || created.apgic_deletes_ledger || !created.ledger_retained || !created.profile_erased) {
        throw new Error("Удаление не должно быть деактивацией и не должно уничтожать запись учёта.");
      }
      setDeletion(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Учётная запись не удалена.");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="journey">
      <form className="panel" onSubmit={interpret}>
        <label htmlFor="request">С чем нужна помощь</label>
        <textarea
          id="request"
          name="request"
          rows={4}
          required
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder="Например: тревожно перед выступлениями, плохо сплю"
        />
        <button type="submit" disabled={pending}>Разобрать запрос</button>
      </form>

      {error ? <p className="alert" role="alert">{error}</p> : null}
      {pending ? <p role="status">Сохраняем шаг…</p> : null}

      {intent ? (
        <form className="panel" onSubmit={confirm}>
          <h2>Проверьте, как мы поняли запрос</h2>
          <p>{intent.notice}</p>
          <p className="meta">
            Диагноз не поставлен: {intent.diagnosis_asserted ? "да" : "нет"}. Каталог: {intent.catalog_mode}.
          </p>
          <fieldset>
            <legend>Темы, которые можно исправить</legend>
            {TOPICS.map((topic) => (
              <label key={topic.id} className="check">
                <input
                  type="checkbox"
                  name="topics"
                  value={topic.id}
                  checked={topics.includes(topic.id)}
                  onChange={() => toggleTopic(topic.id)}
                />
                {topic.label}
              </label>
            ))}
          </fieldset>
          <button type="submit" disabled={pending}>Подтвердить и показать специалистов</button>
        </form>
      ) : null}

      {matches ? (
        <section className="panel" aria-labelledby="matches-title">
          <h2 id="matches-title">Доступные специалисты</h2>
          {matches.length === 0 ? <p>По подтверждённым темам сейчас нет подходящих специалистов.</p> : null}
          <ul className="cards">
            {matches.map((card) => (
              <li key={card.specialist_id}>
                <h3>{card.display_name}</h3>
                <p>{PROFESSIONS[card.profession] ?? card.profession} · {card.format === "ONLINE" ? "онлайн" : card.format}</p>
                <p>{money(card.price_minor, card.currency)} за сессию</p>
                {card.sponsored ? <p>Спонсируемое размещение. Квалификация не обходится.</p> : null}
                <button type="button" onClick={() => choose(card)} disabled={pending}>
                  Выбрать время у {card.display_name}
                </button>
              </li>
            ))}
          </ul>
          {search ? (
            <>
              <p>{search.notice}</p>
              <p>{search.entries.length === 0 ? "В проекции никого нет." : `В проекции: ${search.entries.map((entry) => entry.display_name).join(", ")}.`}</p>
              <p>Индекс владеет допуском: {search.owns_qualification ? "да" : "нет"}.</p>
              <button type="button" onClick={staleSearch} disabled={pending}>Сбросить поисковый индекс</button>
              <button type="button" onClick={rebuildSearch} disabled={pending}>Восстановить поиск из каталога</button>
            </>
          ) : null}
        </section>
      ) : null}

      {specialist ? (
        <section className="panel" aria-labelledby="slots-title">
          <h2 id="slots-title">Эксклюзивные слоты: {specialist.display_name}</h2>
          <ul className="cards">
            {slots.map((slot) => (
              <li key={slot.id}>
                <p>{when(slot.starts_at)} – {when(slot.ends_at)}</p>
                <p>{slot.exclusive ? "Слот эксклюзивный: его может удержать только один клиент." : "Слот не эксклюзивный."}</p>
                <button type="button" onClick={() => acquire(slot)} disabled={pending}>
                  Удержать слот {when(slot.starts_at)}
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {hold ? (
        <section className="panel result" aria-labelledby="hold-title">
          <h2 id="hold-title">Слот удерживается</h2>
          <p>Бронь {hold.booking_id} в состоянии {hold.booking_state}. Удержание {hold.state} до {when(hold.expires_at)}.</p>
          <h3>Оплата у внешнего провайдера</h3>
          <p>APGIC не принимает деньги. Получатель — специалист, исполнение — внешний провайдер.</p>
          <ul className="cards">
            {options.map((option) => (
              <li key={option.method_code}>
                <p>{METHOD_LABELS[option.method_code] ?? option.method_code} · {money(option.amount_minor, option.currency)}</p>
                <button
                  type="button"
                  disabled={pending || option.apgic_accepts_funds || option.execution_owner !== "EXTERNAL_PROVIDER"}
                  onClick={() => chooseMethod(option.method_code)}
                >
                  Выбрать {METHOD_LABELS[option.method_code] ?? option.method_code}
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {instruction ? (
        <section className="panel result" aria-labelledby="pay-title">
          <h2 id="pay-title">Поручение на оплату создано</h2>
          <p>{instruction.notice}</p>
          <p>Заказ {instruction.order_id}, состояние брони {instruction.booking_state}. Провайдер {instruction.provider_id}, способ {instruction.method_code}, сумма {money(instruction.amount_minor, instruction.currency)}.</p>
          <p>Получатель денег: {instruction.payment_recipient_id}. APGIC принимает деньги: {instruction.apgic_accepts_funds ? "да" : "нет"}.</p>
          <button type="button" onClick={captureProviderEvent} disabled={pending}>
            Зафиксировать подтверждение внешнего провайдера
          </button>
        </section>
      ) : null}

      {evidence ? (
        <section className="panel result" aria-labelledby="evidence-title">
          <h2 id="evidence-title">Бронь подтверждена провайдером</h2>
          <p>{evidence.notice}</p>
          <p>Состояние брони {evidence.booking_state}. Запись учёта {evidence.ledger_entry_id}. Получатель {evidence.credit_account_ref}.</p>
          <p>APGIC принимает деньги: {evidence.apgic_accepts_funds ? "да" : "нет"}. Повтор: {evidence.idempotent ? "уже учтён" : "нет"}.</p>
          {fulfillment ? (
            <>
              <h3>Уведомление и вход</h3>
              <p>{fulfillment.notice}</p>
              <p>{fulfillment.join}</p>
            </>
          ) : null}
          <h3>Консультация</h3>
          <p>Статус завершена ставится только по доказательству провайдера связи. APGIC комнатой не владеет и повторно не списывает деньги.</p>
          <button type="button" onClick={recordPresence} disabled={pending}>Зафиксировать факты входа</button>
          <button type="button" onClick={() => reportFailure(true)} disabled={pending}>Сообщить о сбое связи</button>
          <button type="button" onClick={succeedRecovery} disabled={pending}>Восстановление удалось</button>
          <button type="button" onClick={() => reportFailure(false)} disabled={pending}>Сбой без восстановления</button>
          <button type="button" onClick={completeWithoutEvidence} disabled={pending}>Завершить без доказательства</button>
          <button type="button" onClick={completeWithEvidence} disabled={pending}>Завершить по доказательству провайдера</button>
          <button type="button" onClick={exportToGrowth} disabled={pending}>Передать сырую запись в рост</button>
          {session ? (
            <p>
              Сессия {session.state}. {session.notice} Повторное списание: {session.charged_again ? "да" : "нет"}. Сырая запись в деле: {session.raw_content_stored ? "да" : "нет"}.
              {session.refund_path_opened ? " Путь возврата открыт у внешнего провайдера." : ""}
              {session.apgic_returns_funds ? " APGIC возвращает деньги: да." : ""}
            </p>
          ) : null}
          <button type="button" onClick={cancelBooking} disabled={pending}>Отменить бронь через внешнего провайдера</button>
        </section>
      ) : null}

      {cancellation ? (
        <section className="panel result" aria-labelledby="cancel-title">
          <h2 id="cancel-title">Бронь отменена</h2>
          <p>{cancellation.notice}</p>
          <p>Состояние брони {cancellation.booking_state}. Возврат {cancellation.refund_state} у провайдера {cancellation.provider_id}.</p>
          <p>Исходная запись {cancellation.original_ledger_id} сохранена. Запись возврата {cancellation.reversal_ledger_id}.</p>
          <p>APGIC принимает деньги: {cancellation.apgic_accepts_funds ? "да" : "нет"}. APGIC возвращает деньги: {cancellation.apgic_returns_funds ? "да" : "нет"}. Повтор: {cancellation.idempotent ? "уже учтён" : "нет"}.</p>
        </section>
      ) : null}

      {intent ? (
        <section className="panel" aria-labelledby="deletion-title">
          <h2 id="deletion-title">Удаление учётной записи</h2>
          <p>Это удаление, не деактивация. Профиль стирается у внешнего провайдера. Запись учёта оплаты сохраняется.</p>
          <button type="button" onClick={deleteAccount} disabled={pending}>Удалить учётную запись</button>
          {deletion ? (
            <>
              <p>{deletion.notice}</p>
              <p>Состояние {deletion.state}. Деактивация: {deletion.deactivation ? "да" : "нет"}.</p>
              <p>Профиль стёрт: {deletion.profile_erased ? "да" : "нет"}. Запись учёта сохранена: {deletion.ledger_retained ? "да" : "нет"}.</p>
              <p>APGIC уничтожает запись учёта: {deletion.apgic_deletes_ledger ? "да" : "нет"}. Повтор: {deletion.idempotent ? "уже учтён" : "нет"}.</p>
            </>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
