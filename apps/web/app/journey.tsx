"use client";

import { FormEvent, useEffect, useState } from "react";

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

const QUICK_STARTS = [
  { label: "Тревога и стресс", example: "Мне тревожно перед выступлениями" },
  { label: "Проблемы со сном", example: "Мне сложно засыпать и восстанавливаться после рабочего дня. Хочу улучшить сон." },
  { label: "Отношения", example: "Мне непросто выстраивать отношения и договариваться с близкими. Хочу обсудить ситуацию." },
  { label: "Работа и карьера", example: "Не понимаю, куда двигаться в работе. Хочу разобраться в карьерных целях." },
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

function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}

const JOURNEY_STEPS = [
  { id: 1, label: "Запрос", hint: "Расскажите, что происходит" },
  { id: 2, label: "Подбор", hint: "Сравните специалистов" },
  { id: 3, label: "Время", hint: "Выберите удобный слот" },
  { id: 4, label: "Оплата", hint: "Оплатите у провайдера" },
  { id: 5, label: "Консультация", hint: "Подключитесь в назначенное время" },
] as const;

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
  const [conformanceProviderEvents, setConformanceProviderEvents] = useState(false);
  useEffect(() => {
    let active = true;
    fetch("/v1/meta", { cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject(new Error("meta unavailable")))
      .then((meta: { conformance_provider_events?: boolean }) => {
        if (active) setConformanceProviderEvents(meta.conformance_provider_events === true);
      })
      .catch(() => {
        if (active) setConformanceProviderEvents(false);
      });
    return () => { active = false; };
  }, []);
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
    // A new request invalidates the whole previous booking journey, even if
    // interpreting the replacement request fails.
    setIntent(null);
    setTopics([]);
    setMatches(null);
    setSearch(null);
    setSpecialist(null);
    setSlots([]);
    setHold(null);
    setOptions([]);
    setInstruction(null);
    setEvidence(null);
    setCancellation(null);
    setFulfillment(null);
    setSession(null);
    setDeletion(null);
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
    // Never leave matches from the previous confirmation actionable while
    // the changed topics are being confirmed, or after a failed confirmation.
    setMatches(null);
    setSearch(null);
    setSpecialist(null);
    setSlots([]);
    setHold(null);
    setOptions([]);
    setInstruction(null);
    setEvidence(null);
    setCancellation(null);
    setFulfillment(null);
    setSession(null);
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
    // Changing the specialist invalidates all downstream booking and payment UI.
    // Even a failed slots lookup must not expose the prior specialist's checkout.
    setSpecialist(null);
    setSlots([]);
    setHold(null);
    setOptions([]);
    setInstruction(null);
    setEvidence(null);
    setCancellation(null);
    setFulfillment(null);
    setSession(null);
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
    // A new slot attempt must not leave the previous payment instruction
    // or confirmed booking visible after an unsuccessful hold.
    setHold(null);
    setOptions([]);
    setInstruction(null);
    setEvidence(null);
    setCancellation(null);
    setFulfillment(null);
    setSession(null);
    try {
      const created = await postJSON<Hold>("/v1/slot-holds", {
        help_intent_id: intent.id,
        slot_id: slot.id,
      });
      setHold(created);
      setInstruction(null);
      if (!conformanceProviderEvents) {
        setOptions([]);
        return;
      }
      const listed = await fetch(`/v1/slot-holds/${created.id}/checkout-options`);
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
        const listed = await fetch(`/v1/bookings/${created.booking_id}/fulfillment`);
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
      const created = await postJSON<{ raw_content_included: boolean }>("/v1/consultations/" + evidence.booking_id + "/growth-export", {});
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
      }>("/v1/account-deletions", { source: "WEB" });
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

  const currentStep = evidence ? 5 : hold ? 4 : specialist ? 3 : matches ? 2 : 1;
  const selectedTopicLabels = TOPICS
    .filter((topic) => topics.includes(topic.id))
    .map((topic) => topic.label);

  return (
    <div className="journey journey-product">
      <nav className="journey-progress" aria-label="Этапы записи">
        <ol>
          {JOURNEY_STEPS.map((step) => {
            const done = currentStep > step.id;
            const current = currentStep === step.id;
            return (
              <li
                key={step.id}
                className={done ? "is-done" : current ? "is-current" : "is-upcoming"}
                aria-current={current ? "step" : undefined}
              >
                <span className="journey-step-number" aria-hidden="true">{done ? "✓" : step.id}</span>
                <span>
                  <strong>{step.label}</strong>
                  <small>{step.hint}</small>
                </span>
              </li>
            );
          })}
        </ol>
      </nav>

      <section className="journey-stage journey-stage-primary" aria-labelledby="request-stage-title">
        <div className="journey-stage-heading">
          <span className="journey-stage-kicker">Шаг 1</span>
          <div>
            <h2 id="request-stage-title">Начнём с того, что сейчас важно</h2>
            <p>Опишите ситуацию обычными словами. APGIC сначала покажет своё понимание — решение всегда остаётся за вами.</p>
          </div>
        </div>
        <form className="journey-request-form" onSubmit={interpret}>
          <div className="quick-start-block" aria-label="Быстрый выбор темы">
            <p className="quick-start-caption">Не знаете, с чего начать? Выберите близкую тему:</p>
            <div className="quick-start-options">
              {QUICK_STARTS.map((suggestion) => (
                <button
                  className="quick-start-option"
                  key={suggestion.label}
                  type="button"
                  disabled={pending}
                  aria-pressed={text === suggestion.example}
                  onClick={() => {
                    setText(suggestion.example);
                    document.getElementById("request")?.focus();
                  }}
                >
                  {suggestion.label}
                </button>
              ))}
            </div>
            <p className="quick-start-help">Мы подставим пример. Вы сможете изменить его перед отправкой — ничего не отправляется автоматически.</p>
          </div>
          <label htmlFor="request">С чем нужна помощь</label>
          <textarea
            id="request"
            name="request"
            rows={5}
            required
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="Например: тревожно перед выступлениями, плохо сплю и сложно сосредоточиться"
          />
          <div className="journey-form-footer">
            <span>Регистрация для начала подбора не нужна</span>
            <button type="submit" disabled={pending}>Разобрать запрос</button>
          </div>
        </form>
      </section>

      {error ? <p className="alert journey-alert" role="alert">{error}</p> : null}
      {pending ? <p className="journey-saving" role="status">Сохраняем шаг…</p> : null}

      {intent ? (
        <section className="journey-stage" aria-labelledby="intent-stage-title">
          <div className="journey-stage-heading">
            <span className="journey-stage-kicker">Проверьте смысл</span>
            <div>
              <h2 id="intent-stage-title">Правильно ли мы вас поняли?</h2>
              <p>{intent.notice}</p>
            </div>
          </div>

          <div className="journey-trust-note">
            <span aria-hidden="true">✓</span>
            <div>
              <strong>Это не диагноз</strong>
              <p className="meta">Диагноз не поставлен: {intent.diagnosis_asserted ? "да" : "нет"}.</p>
            </div>
          </div>

          <form onSubmit={confirm}>
            <fieldset className="journey-topics">
              <legend>Уточните темы запроса</legend>
              <div className="journey-topic-grid">
                {TOPICS.map((topic) => (
                  <label key={topic.id} className="check journey-topic">
                    <input
                      type="checkbox"
                      name="topics"
                      value={topic.id}
                      checked={topics.includes(topic.id)}
                      onChange={() => toggleTopic(topic.id)}
                    />
                    <span>{topic.label}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="journey-form-footer">
              <span>{selectedTopicLabels.length ? selectedTopicLabels.join(" · ") : "Выберите хотя бы одну тему"}</span>
              <button type="submit" disabled={pending || topics.length === 0}>Подтвердить и показать специалистов</button>
            </div>
          </form>
        </section>
      ) : null}

      {matches ? (
        <section className="journey-stage" aria-labelledby="matches-title">
          <div className="journey-stage-heading">
            <span className="journey-stage-kicker">Шаг 2</span>
            <div>
              <h2 id="matches-title">Специалисты под ваш запрос</h2>
              <p>
                {matches.length
                  ? `Нашли ${matches.length} ${matches.length === 1 ? "подходящий вариант" : "подходящих варианта"}. Сравните формат, стоимость и доступное время.`
                  : "По подтверждённым темам сейчас нет подходящих специалистов."}
              </p>
            </div>
          </div>

          {matches.length ? (
            <ul className="specialist-cards">
              {matches.map((card) => (
                <li key={card.specialist_id} className={specialist?.specialist_id === card.specialist_id ? "is-selected" : ""}>
                  <div className="specialist-card-top">
                    <div className="specialist-avatar" aria-hidden="true">{initials(card.display_name)}</div>
                    <div className="specialist-card-name">
                      <span className="match-badge">Подобран по подтверждённым темам</span>
                      <h3>{card.display_name}</h3>
                      <p>{PROFESSIONS[card.profession] ?? card.profession}</p>
                    </div>
                  </div>
                  <div className="specialist-card-facts">
                    <span>{card.format === "ONLINE" ? "Онлайн" : card.format}</span>
                    <span>{money(card.price_minor, card.currency)} / сессия</span>
                    {card.sponsored ? <span>Спонсируемое · без обхода квалификации</span> : null}
                  </div>
                  <button type="button" onClick={() => choose(card)} disabled={pending}>
                    Выбрать время у {card.display_name}
                  </button>
                </li>
              ))}
            </ul>
          ) : null}

          {search ? (
            <details className="journey-proof-tools">
              <summary>Проверка каталога и поисковой проекции</summary>
              <div className="journey-proof-body">
                <p>{search.notice}</p>
                <p>{search.entries.length === 0 ? "В проекции никого нет." : `В проекции: ${search.entries.map((entry) => entry.display_name).join(", ")}.`}</p>
                <p>Индекс владеет допуском: {search.owns_qualification ? "да" : "нет"}.</p>
                <div className="journey-inline-actions">
                  <button type="button" onClick={staleSearch} disabled={pending}>Сбросить поисковый индекс</button>
                  <button type="button" onClick={rebuildSearch} disabled={pending}>Восстановить поиск из каталога</button>
                </div>
              </div>
            </details>
          ) : null}
        </section>
      ) : null}

      {specialist ? (
        <section className="journey-stage" aria-labelledby="slots-title">
          <div className="journey-stage-heading">
            <span className="journey-stage-kicker">Шаг 3</span>
            <div>
              <h2 id="slots-title">Выберите время с {specialist.display_name}</h2>
              <p>Слот резервируется эксклюзивно на короткое время, чтобы его не занял другой клиент во время оформления.</p>
            </div>
          </div>
          <div className="selected-specialist-strip">
            <div className="specialist-avatar compact" aria-hidden="true">{initials(specialist.display_name)}</div>
            <div>
              <strong>{specialist.display_name}</strong>
              <span>{PROFESSIONS[specialist.profession] ?? specialist.profession} · {money(specialist.price_minor, specialist.currency)}</span>
            </div>
          </div>
          <ul className="slot-grid">
            {slots.map((slot) => (
              <li key={slot.id}>
                <span className="slot-day">{when(slot.starts_at).split(",")[0]}</span>
                <strong>{when(slot.starts_at)}</strong>
                <small>{slot.exclusive ? "Эксклюзивный слот" : "Доступное время"}</small>
                <button type="button" onClick={() => acquire(slot)} disabled={pending}>
                  Удержать слот {when(slot.starts_at)}
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {hold ? (
        <section className="journey-stage result booking-checkout" aria-labelledby="hold-title">
          <div className="journey-stage-heading">
            <span className="journey-stage-kicker">Шаг 4</span>
            <div>
              <h2 id="hold-title">Слот удерживается</h2>
              <p>{conformanceProviderEvents
                ? "Время временно закреплено за вами. Дальнейшие шаги доступны только в тестовом окружении."
                : "Время временно удерживается, но запись ещё не подтверждена. Оплата через внешнего провайдера сейчас недоступна; удержание может истечь без подтверждения."}</p>
            </div>
          </div>
          <div className="booking-summary">
            <div><span>Бронь</span><strong>{hold.booking_id}</strong></div>
            <div><span>Статус</span><strong>{hold.booking_state}</strong></div>
            <div><span>Удержание до</span><strong>{when(hold.expires_at)}</strong></div>
          </div>
          <p className="meta">Бронь {hold.booking_id} в состоянии {hold.booking_state}. Удержание {hold.state} до {when(hold.expires_at)}.</p>

          <div className="payment-boundary">
            <span aria-hidden="true">↗</span>
            <div>
              <strong>Оплата проходит у внешнего провайдера</strong>
              <p>APGIC не принимает деньги. Получатель — специалист, исполнение — внешний провайдер.</p>
            </div>
          </div>

          {conformanceProviderEvents ? (
          <div className="payment-options" aria-label="Тестовые способы оплаты">
            {options.map((option) => (
              <article key={option.method_code}>
                <div>
                  <strong>{METHOD_LABELS[option.method_code] ?? option.method_code}</strong>
                  <span>{money(option.amount_minor, option.currency)}</span>
                </div>
                <button
                  type="button"
                  disabled={pending || option.apgic_accepts_funds || option.execution_owner !== "EXTERNAL_PROVIDER"}
                  onClick={() => chooseMethod(option.method_code)}
                >
                  Выбрать {METHOD_LABELS[option.method_code] ?? option.method_code}
                </button>
              </article>
            ))}
          </div>
          ) : (
            <p className="journey-payment-unavailable" role="status">
              Онлайн-оплата через внешнего исполнителя пока недоступна.
              Карта и СБП не подключены для реальных платежей. APGIC не принимает деньги.
              Удержание времени не является оплаченной или подтверждённой записью.
            </p>
          )}
        </section>
      ) : null}

      {instruction ? (
        <section className="journey-stage payment-status" aria-labelledby="pay-title">
          <div className="payment-status-icon" aria-hidden="true">…</div>
          <div>
            <span className="journey-stage-kicker">Ожидаем провайдера</span>
            <h2 id="pay-title">Поручение на оплату создано</h2>
            <p>{instruction.notice}</p>
            <p className="meta">Заказ {instruction.order_id} · {instruction.booking_state} · {money(instruction.amount_minor, instruction.currency)}</p>
            <p className="meta">APGIC принимает деньги: {instruction.apgic_accepts_funds ? "да" : "нет"}.</p>
          </div>
          {conformanceProviderEvents ? (
            <button type="button" onClick={captureProviderEvent} disabled={pending}>
              Зафиксировать подтверждение внешнего провайдера (только тест)
            </button>
          ) : (
            <p role="status">Оплата и её подтверждение выполняются только внешним исполнителем. APGIC не принимает деньги и не может самостоятельно подтвердить оплату. Пока подтверждение не получено, запись не считается оплаченной.</p>
          )}
        </section>
      ) : null}

      {evidence ? (
        <section className="journey-stage result consultation-stage" aria-labelledby="evidence-title">
          <div className="consultation-success">
            <span className="success-mark" aria-hidden="true">✓</span>
            <div>
              <span className="journey-stage-kicker">Шаг 5</span>
              <h2 id="evidence-title">Бронь подтверждена провайдером</h2>
              <p>{evidence.notice}</p>
            </div>
          </div>
          <div className="booking-summary">
            <div><span>Бронь</span><strong>{evidence.booking_state}</strong></div>
            <div><span>Учёт</span><strong>{evidence.ledger_entry_id}</strong></div>
            <div><span>Повтор</span><strong>{evidence.idempotent ? "уже учтён" : "нет"}</strong></div>
          </div>
          <p className="meta">APGIC принимает деньги: {evidence.apgic_accepts_funds ? "да" : "нет"}. Повтор: {evidence.idempotent ? "уже учтён" : "нет"}.</p>

          {fulfillment ? (
            <div className="consultation-access">
              <div><span>Уведомление</span><p>{fulfillment.notice}</p></div>
              <div><span>Вход в консультацию</span><p>{fulfillment.join}</p></div>
            </div>
          ) : null}

          <div className="consultation-actions">
            <div>
              <h4>Консультация</h4>
              <p>Статус меняется только по подтверждённым фактам провайдера связи. Повторная оплата при восстановлении не создаётся.</p>
            </div>
            <div className="journey-inline-actions">
              <button type="button" onClick={recordPresence} disabled={pending}>Зафиксировать факты входа</button>
              <button type="button" onClick={() => reportFailure(true)} disabled={pending}>Сообщить о сбое связи</button>
              <button type="button" onClick={succeedRecovery} disabled={pending}>Восстановление удалось</button>
              <button type="button" onClick={() => reportFailure(false)} disabled={pending}>Сбой без восстановления</button>
              <button type="button" onClick={completeWithEvidence} disabled={pending}>Завершить по доказательству провайдера</button>
            </div>
          </div>

          {session ? (
            <div className="session-status" role="status">
              <strong>Сессия {session.state}.</strong>
              <span>{session.notice}</span>
              <span>Повторное списание: {session.charged_again ? "да" : "нет"}.</span>
              <span>Сырая запись в деле: {session.raw_content_stored ? "да" : "нет"}.</span>
              {session.refund_path_opened ? <span>Путь возврата открыт у внешнего провайдера.</span> : null}
              {session.apgic_returns_funds ? <span>APGIC возвращает деньги: да.</span> : null}
            </div>
          ) : null}

          <button className="secondary-danger-action" type="button" onClick={cancelBooking} disabled={pending}>
            Отменить бронь через внешнего провайдера
          </button>

          <details className="journey-proof-tools">
            <summary>Служебные проверки безопасности</summary>
            <div className="journey-proof-body">
              <p>Эти действия нужны для автоматической проверки fail-closed сценариев и не являются частью обычного пути клиента.</p>
              <div className="journey-inline-actions">
                <button type="button" onClick={completeWithoutEvidence} disabled={pending}>Завершить без доказательства</button>
                <button type="button" onClick={exportToGrowth} disabled={pending}>Передать сырую запись в рост</button>
              </div>
            </div>
          </details>
        </section>
      ) : null}

      {cancellation ? (
        <section className="journey-stage result cancellation-result" aria-labelledby="cancel-title">
          <span className="journey-stage-kicker">Отмена и возврат</span>
          <h2 id="cancel-title">Бронь отменена</h2>
          <p>{cancellation.notice}</p>
          <div className="booking-summary">
            <div><span>Бронь</span><strong>{cancellation.booking_state}</strong></div>
            <div><span>Возврат</span><strong>{cancellation.refund_state}</strong></div>
            <div><span>Провайдер</span><strong>{cancellation.provider_id}</strong></div>
          </div>
          <p>Исходная запись {cancellation.original_ledger_id} сохранена. Запись возврата {cancellation.reversal_ledger_id}.</p>
          <p>APGIC принимает деньги: {cancellation.apgic_accepts_funds ? "да" : "нет"}. APGIC возвращает деньги: {cancellation.apgic_returns_funds ? "да" : "нет"}. Повтор: {cancellation.idempotent ? "уже учтён" : "нет"}.</p>
        </section>
      ) : null}

      {intent ? (
        <details className="journey-account-tools">
          <summary>Управление учётной записью</summary>
          <section aria-labelledby="deletion-title">
            <h2 id="deletion-title">Удаление учётной записи</h2>
            <p>Профиль стирается, а обязательная финансовая и audit-история сохраняется по правилам хранения.</p>
            <button type="button" onClick={deleteAccount} disabled={pending}>Удалить учётную запись</button>
            {deletion ? (
              <div className="deletion-result">
                <p>{deletion.notice}</p>
                <p>Состояние {deletion.state}. Деактивация: {deletion.deactivation ? "да" : "нет"}.</p>
                <p>Профиль стёрт: {deletion.profile_erased ? "да" : "нет"}. Запись учёта сохранена: {deletion.ledger_retained ? "да" : "нет"}.</p>
                <p>APGIC уничтожает запись учёта: {deletion.apgic_deletes_ledger ? "да" : "нет"}. Повтор: {deletion.idempotent ? "уже учтён" : "нет"}.</p>
              </div>
            ) : null}
          </section>
        </details>
      ) : null}
    </div>
  );
}
