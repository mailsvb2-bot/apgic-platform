"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FinanceOpenDiscrepancy, FinanceOpenDiscrepancyList } from "../../../packages/contracts/src/generated/apgic-v1";

type ResolutionKind = "PROVIDER_CORRECTION_CONFIRMED" | "SUPERSEDED_BY_MATCHED_RECONCILIATION" | "OPERATIONAL_EXCEPTION_ACCEPTED";
const RESOLUTION_LABELS: Record<ResolutionKind, string> = {
  PROVIDER_CORRECTION_CONFIRMED: "Исправление реестра подтверждено",
  SUPERSEDED_BY_MATCHED_RECONCILIATION: "Заменено успешной повторной сверкой",
  OPERATIONAL_EXCEPTION_ACCEPTED: "Операционное исключение принято",
};
const KIND_LABELS: Record<FinanceOpenDiscrepancy["kind"], string> = {
  PROVIDER_LEDGER_MISMATCH: "Расхождение с реестром провайдера",
  STATEMENT_MISSING: "Не найдена строка выписки",
};

function isDiscrepancy(value: unknown): value is FinanceOpenDiscrepancy {
  if (!value || typeof value !== "object") return false;
  const row = value as Partial<FinanceOpenDiscrepancy>;
  return typeof row.discrepancy_id === "string" &&
    (row.kind === "PROVIDER_LEDGER_MISMATCH" || row.kind === "STATEMENT_MISSING") &&
    typeof row.currency === "string" &&
    typeof row.expected_fee_minor === "number" &&
    typeof row.provider_fee_minor === "number" &&
    typeof row.expected_gross_minor === "number" &&
    typeof row.provider_gross_minor === "number" &&
    typeof row.created_at === "string" &&
    typeof row.provider_instance_id === "string" &&
    typeof row.settlement_instruction_id === "string" &&
    Array.isArray(row.evidence_refs) &&
    row.evidence_refs.every((item) => typeof item === "string");
}

function isList(value: unknown): value is FinanceOpenDiscrepancyList {
  return !!value && typeof value === "object" &&
    Array.isArray((value as Partial<FinanceOpenDiscrepancyList>).items) &&
    (value as FinanceOpenDiscrepancyList).items.every(isDiscrepancy);
}

function money(minor: number, currency: string): string {
  return new Intl.NumberFormat("ru-RU", { style: "currency", currency }).format(minor / 100);
}

export default function FinanceOperationsQueue() {
  const [items, setItems] = useState<FinanceOpenDiscrepancy[]>([]);
  const [state, setState] = useState<"IDLE" | "LOADING" | "READY" | "DENIED" | "ERROR">("IDLE");
  const [selectedID, setSelectedID] = useState("");
  const [kind, setKind] = useState<ResolutionKind>("PROVIDER_CORRECTION_CONFIRMED");
  const [reason, setReason] = useState("");
  const [evidenceText, setEvidenceText] = useState("");
  const [providerEvidenceID, setProviderEvidenceID] = useState("");
  const [supersedingResultID, setSupersedingResultID] = useState("");
  const [idempotencyKey, setIdempotencyKey] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [resultMessage, setResultMessage] = useState("");
  const [totp, setTotp] = useState("");
  const [mfaState, setMfaState] = useState<"IDLE" | "LOADING" | "VERIFIED" | "ERROR">("IDLE");
  const inFlight = useRef(false);
  const loadEpoch = useRef(0);
  const mounted = useRef(true);

  const refresh = useCallback(async () => {
    const epoch = ++loadEpoch.current;
    setState("LOADING");
    try {
      const response = await fetch("/v1/finance/operations/discrepancies?limit=100", {
        method: "GET",
        credentials: "include",
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      if (!mounted.current || epoch !== loadEpoch.current) return;
      if (response.status === 401 || response.status === 403) {
        setItems([]);
        setState("DENIED");
        return;
      }
      if (!response.ok) throw new Error("finance operations unavailable");
      const payload: unknown = await response.json();
      if (!mounted.current || epoch !== loadEpoch.current) return;
      if (!isList(payload)) throw new Error("finance operations contract invalid");
      setItems(payload.items);
      setSelectedID(previous => payload.items.some(item => item.discrepancy_id === previous) ? previous : "");
      setState("READY");
    } catch {
      if (!mounted.current || epoch !== loadEpoch.current) return;
      setItems([]);
      setState("ERROR");
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void refresh();
    return () => { mounted.current = false; loadEpoch.current += 1; };
  }, [refresh]);

  const selected = items.find(item => item.discrepancy_id === selectedID);
  const evidenceRefs = evidenceText.split("\n").map(value => value.trim()).filter(Boolean);
  const valid = Boolean(selected && reason.trim().length >= 8 && evidenceRefs.length > 0 &&
    (kind !== "PROVIDER_CORRECTION_CONFIRMED" || providerEvidenceID.trim()) &&
    (kind !== "SUPERSEDED_BY_MATCHED_RECONCILIATION" || supersedingResultID.trim()));
  const invalidateKey = () => setIdempotencyKey("");
  const select = (id: string) => {
    setSelectedID(id);
    setKind("PROVIDER_CORRECTION_CONFIRMED");
    setReason("");
    setEvidenceText("");
    setProviderEvidenceID("");
    setSupersedingResultID("");
    invalidateKey();
    setResultMessage("");
  };
  async function verifySecondFactor() {
    if (!/^\\d{6}$/.test(totp) || mfaState === "LOADING") return;
    setMfaState("LOADING");
    try {
      const response = await fetch("/v1/finance/mfa/verify", {
        method: "POST", credentials: "include", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-APGIC-MFA": "totp-v1" },
        body: JSON.stringify({ code: totp }),
      });
      if (!mounted.current) return;
      setTotp("");
      if (response.status === 204) {
        setMfaState("VERIFIED");
        setResultMessage("Второй фактор проверен сервером. Подтверждение действует не более 10 минут и только в этой сессии.");
      } else {
        setMfaState("ERROR");
        setResultMessage("Подтверждение отклонено. Проверьте код и наличие заранее выданного фактора.");
      }
    } catch {
      if (mounted.current) {
        setMfaState("ERROR");
        setResultMessage("Не удалось связаться с сервером MFA; финансовое решение не выполнено.");
      }
    }
  }

  async function resolve() {
    if (!valid || !selected || inFlight.current) return;
    inFlight.current = true;
    setSubmitting(true);
    setResultMessage("");
    // Preserve the key after network uncertainty so retry cannot create a
    // second economic decision. Never send client-supplied operator/audit IDs.
    const key = idempotencyKey || crypto.randomUUID();
    setIdempotencyKey(key);
    try {
      const response = await fetch(
        `/v1/finance/operations/discrepancies/${encodeURIComponent(selected.discrepancy_id)}/resolutions`,
        {
          method: "POST", credentials: "include", cache: "no-store",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({
            idempotency_key: key, kind, reason: reason.trim(), evidence_refs: evidenceRefs,
            ...(kind === "PROVIDER_CORRECTION_CONFIRMED" ? { provider_evidence_id: providerEvidenceID.trim() } : {}),
            ...(kind === "SUPERSEDED_BY_MATCHED_RECONCILIATION" ? { superseding_result_id: supersedingResultID.trim() } : {}),
          }),
        },
      );
      if (!mounted.current) return;
      if (response.ok) {
        select("");
        await refresh();
        if (mounted.current) setResultMessage("Решение записано сервером; очередь обновлена.");
      } else {
        setResultMessage(response.status === 428
          ? "Для действия требуется свежий второй фактор. Проверьте код TOTP ниже; решение не записано."
          : response.status === 401 || response.status === 403
            ? "Нет действующего полномочия финансового оператора."
            : response.status === 409
              ? "Конфликт решения или ключа. Обновите очередь перед новым решением."
              : "Сервер не подтвердил запись решения. Исправьте данные либо повторите с тем же ключом.");
      }
    } catch {
      if (mounted.current) setResultMessage("Сетевой результат неизвестен. Повтор сохранит ключ идемпотентности.");
    } finally {
      inFlight.current = false;
      if (mounted.current) setSubmitting(false);
    }
  }

  return (
    <main className="journey-stage" aria-label="Финансовые расхождения">
      <h1>Финансовая сверка</h1>
      <p>Очередь показывает только расхождения, подтверждённые серверной сверкой с платёжным провайдером. APGIC не хранит и не переводит деньги.</p>
      <button type="button" onClick={() => void refresh()} disabled={state === "LOADING"}>
        {state === "LOADING" ? "Обновляем…" : "Обновить очередь"}
      </button>
      {state === "DENIED" ? <p role="alert">Доступ запрещён. Нужна действующая сессия финансового оператора с выданным полномочием.</p> : null}
      {state === "ERROR" ? <p role="alert">Не удалось получить очередь с сервера. Данные не считаются актуальными.</p> : null}
      {state === "READY" && items.length === 0 ? <p role="status">Открытых расхождений нет.</p> : null}
      {state === "READY" && items.length > 0 ? (
        <section aria-label="Открытые расхождения">
          <p>Открыто расхождений: {items.length}</p>
          {items.map((item) => (
            <article key={item.discrepancy_id} className="journey-stage" aria-label={`Расхождение ${item.discrepancy_id}`}>
              <h2>{KIND_LABELS[item.kind]}</h2>
              <p>Создано: {new Date(item.created_at).toLocaleString("ru-RU")}</p>
              <dl>
                <dt>Ожидаемая сумма</dt><dd>{money(item.expected_gross_minor, item.currency)}</dd>
                <dt>Сумма в реестре провайдера</dt><dd>{money(item.provider_gross_minor, item.currency)}</dd>
                <dt>Ожидаемая комиссия</dt><dd>{money(item.expected_fee_minor, item.currency)}</dd>
                <dt>Комиссия провайдера</dt><dd>{money(item.provider_fee_minor, item.currency)}</dd>
                <dt>Провайдер</dt><dd>{item.provider_instance_id}</dd>
                <dt>Инструкция расчёта</dt><dd>{item.settlement_instruction_id}</dd>
                <dt>Доказательства</dt><dd>{item.evidence_refs.join(", ")}</dd>
              </dl>
            </article>
          ))}
        </section>
      ) : null}
      {state === "READY" && items.length > 0 ? (
        <section aria-label="Решение по расхождению">
          <h2>Рассмотреть расхождение</h2>
          <label htmlFor="finance-select-discrepancy">Расхождение</label>
          <select id="finance-select-discrepancy" value={selectedID} disabled={submitting}
            onChange={event => select(event.target.value)}>
            <option value="">Выберите запись из очереди</option>
            {items.map(item => <option key={item.discrepancy_id} value={item.discrepancy_id}>{item.discrepancy_id}</option>)}
          </select>
          {selected ? (
            <>
              <section aria-label="Дополнительное подтверждение финансового оператора">
                <h3>Подтвердить личность (MFA)</h3>
                <p>Введите одноразовый код из предварительно выданного администратором приложения-аутентификатора. Новый фактор здесь не создаётся.</p>
                <label htmlFor="finance-totp">Одноразовый код</label>
                <input id="finance-totp" type="text" inputMode="numeric" autoComplete="one-time-code"
                  maxLength={6} pattern="[0-9]{6}" value={totp} disabled={mfaState === "LOADING"}
                  onChange={event => { setTotp(event.target.value.replace(/\\D/g, "").slice(0, 6)); setMfaState("IDLE"); }} />
                <button type="button" disabled={!/^\\d{6}$/.test(totp) || mfaState === "LOADING"}
                  onClick={() => void verifySecondFactor()}>
                  {mfaState === "LOADING" ? "Проверяем код…" : "Подтвердить второй фактор"}
                </button>
                {mfaState === "VERIFIED" ? <p role="status">MFA подтверждён для текущей сессии.</p> : null}
                {mfaState === "ERROR" ? <p role="alert">MFA не подтверждён.</p> : null}
              </section>
              <p>Инструкция: {selected.settlement_instruction_id}. Провайдер: {selected.provider_instance_id}.</p>
              <label htmlFor="finance-resolution-kind">Тип решения</label>
              <select id="finance-resolution-kind" value={kind} disabled={submitting}
                onChange={event => { setKind(event.target.value as ResolutionKind); invalidateKey(); }}>
                {Object.entries(RESOLUTION_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
              <label htmlFor="finance-resolution-reason">Обоснование</label>
              <textarea id="finance-resolution-reason" value={reason} disabled={submitting}
                onChange={event => { setReason(event.target.value); invalidateKey(); }} />
              <label htmlFor="finance-resolution-refs">Ссылки на доказательства, по одной на строку</label>
              <textarea id="finance-resolution-refs" value={evidenceText} disabled={submitting}
                onChange={event => { setEvidenceText(event.target.value); invalidateKey(); }} />
              {kind === "PROVIDER_CORRECTION_CONFIRMED" ? (
                <>
                  <label htmlFor="finance-provider-evidence">ID нового доказательства провайдера</label>
                  <input id="finance-provider-evidence" value={providerEvidenceID} disabled={submitting}
                    onChange={event => { setProviderEvidenceID(event.target.value); invalidateKey(); }} />
                </>
              ) : null}
              {kind === "SUPERSEDED_BY_MATCHED_RECONCILIATION" ? (
                <>
                  <label htmlFor="finance-superseding-result">ID успешной повторной сверки</label>
                  <input id="finance-superseding-result" value={supersedingResultID} disabled={submitting}
                    onChange={event => { setSupersedingResultID(event.target.value); invalidateKey(); }} />
                </>
              ) : null}
              <button type="button" disabled={!valid || submitting} onClick={() => void resolve()}>
                {submitting ? "Проверяем решение…" : "Записать решение с серверной проверкой"}
              </button>
            </>
          ) : null}
        </section>
      ) : null}
      {resultMessage ? <p role="status">{resultMessage}</p> : null}
      <p>Запись решения требует отдельного полномочия и свежего step-up. Сервер проверяет их при каждой операции; веб-форма не предоставляет обхода. Step-up выдаётся только после проверки заранее зарегистрированного TOTP-фактора, привязанного к текущей серверной сессии.</p>
    </main>
  );
}
