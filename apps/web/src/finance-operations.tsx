"use client";

import { useEffect, useState } from "react";
import type { FinanceOpenDiscrepancy, FinanceOpenDiscrepancyList } from "../../../packages/contracts/src/generated/apgic-v1";

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

  async function refresh() {
    setState("LOADING");
    try {
      const response = await fetch("/v1/finance/operations/discrepancies?limit=100", {
        method: "GET",
        credentials: "include",
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      if (response.status === 401 || response.status === 403) {
        setItems([]);
        setState("DENIED");
        return;
      }
      if (!response.ok) throw new Error("finance operations unavailable");
      const payload: unknown = await response.json();
      if (!isList(payload)) throw new Error("finance operations contract invalid");
      setItems(payload.items);
      setState("READY");
    } catch {
      setItems([]);
      setState("ERROR");
    }
  }

  useEffect(() => { void refresh(); }, []);

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
      <p>Закрытие расхождения требует свежего подтверждения личности (step-up) и отдельного полномочия. Браузерный step-up-процесс пока не подключён; интерфейс намеренно не имитирует его и не предлагает обходной способ закрытия.</p>
    </main>
  );
}
