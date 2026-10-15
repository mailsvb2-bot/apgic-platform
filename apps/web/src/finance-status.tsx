"use client";

import { useEffect, useRef, useState } from "react";
import type { FinanceOrderStatus } from "../../../packages/contracts/src/generated/apgic-v1";

// Read-only projection of R4 provider-executed financial facts. APGIC never
// holds, nets or pays out funds; this view must not imply a balance or a
// payout that the external provider has not evidenced.
const SETTLEMENT_LABELS: Record<FinanceOrderStatus["settlement_status"], string> = {
  NOT_INSTRUCTED: "Расчёт ещё не сформирован",
  HELD: "Расчёт приостановлен до настройки политики или провайдера",
  INSTRUCTED: "Инструкция расчёта передана провайдеру",
  PROVIDER_SETTLED: "Провайдер подтвердил расчёт",
};

const PAYOUT_LABELS: Record<FinanceOrderStatus["payout_status"], string> = {
  NOT_ELIGIBLE: "Выплата недоступна до расчёта",
  AWAITING_PROVIDER_ELIGIBILITY: "Ожидаем подтверждения провайдером условий выплаты",
  BLOCKED: "Выплата заблокирована провайдером или политикой",
  INSTRUCTED: "Инструкция выплаты передана провайдеру",
  PROVIDER_PAID_OUT: "Провайдер подтвердил выплату",
};

const RECONCILIATION_LABELS: Record<FinanceOrderStatus["reconciliation_outcome"], string> = {
  NOT_RECONCILED: "Сверка ещё не проводилась",
  MATCHED: "Сверка с провайдером сошлась",
  MISMATCH: "Обнаружено расхождение; им занимается финансовая служба",
};

function isStatus(value: unknown, orderID: string): value is FinanceOrderStatus {
  if (!value || typeof value !== "object") return false;
  const data = value as Partial<FinanceOrderStatus>;
  return (
    data.order_id === orderID &&
    data.execution_owner === "EXTERNAL_PROVIDER" &&
    typeof data.currency === "string" &&
    typeof data.total_amount_minor === "number" &&
    typeof data.settlement_status === "string" && data.settlement_status in SETTLEMENT_LABELS &&
    typeof data.payout_status === "string" && data.payout_status in PAYOUT_LABELS &&
    typeof data.reconciliation_outcome === "string" && data.reconciliation_outcome in RECONCILIATION_LABELS
  );
}

function money(minor: number, currency: string): string {
  return new Intl.NumberFormat("ru-RU", { style: "currency", currency }).format(minor / 100);
}

export default function FinanceStatusView({ orderID }: { orderID: string }) {
  const [status, setStatus] = useState<FinanceOrderStatus | null>(null);
  const [state, setState] = useState<"IDLE" | "LOADING" | "READY" | "NOT_FOUND" | "DENIED" | "ERROR">("IDLE");
  const epoch = useRef(0);
  useEffect(() => {
    epoch.current += 1;
    setStatus(null);
    setState("IDLE");
    return () => { epoch.current += 1; };
  }, [orderID]);

  async function read() {
    const current = ++epoch.current;
    setState("LOADING");
    setStatus(null);
    try {
      const response = await fetch(`/v1/finance/orders/${encodeURIComponent(orderID)}/status`, {
        credentials: "include",
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      if (current !== epoch.current) return;
      if (response.status === 401 || response.status === 403) { setState("DENIED"); return; }
      if (response.status === 404) { setState("NOT_FOUND"); return; }
      if (!response.ok) { setState("ERROR"); return; }
      const payload: unknown = await response.json();
      if (current !== epoch.current) return;
      if (!isStatus(payload, orderID)) { setState("ERROR"); return; }
      setStatus(payload);
      setState("READY");
    } catch {
      if (current === epoch.current) setState("ERROR");
    }
  }

  return (
    <section className="journey-stage" aria-label="Финансовый статус заказа">
      <h3>Расчёт и выплата</h3>
      <p>Деньги принимает и распределяет платёжный провайдер. APGIC не хранит средства и показывает только подтверждённые провайдером факты.</p>
      <button type="button" onClick={() => void read()} disabled={state === "LOADING"}>
        {state === "LOADING" ? "Проверяем статус…" : "Проверить расчёт и выплату"}
      </button>
      {state === "NOT_FOUND" ? <p role="status">Заказ не найден в вашей сессии.</p> : null}
      {state === "DENIED" ? <p role="alert">Нет подтверждённого доступа. Повторно войдите в свою сессию.</p> : null}
      {state === "ERROR" ? <p role="alert">Не удалось подтвердить статус на сервере. Повторите проверку.</p> : null}
      {state === "READY" && status ? (
        <dl role="status">
          <dt>Сумма заказа</dt><dd>{money(status.total_amount_minor, status.currency)}</dd>
          {typeof status.platform_fee_minor === "number" ? (
            <><dt>Комиссия платформы</dt><dd>{money(status.platform_fee_minor, status.currency)} (политика {status.commission_policy_version})</dd></>
          ) : null}
          {typeof status.beneficiary_amount_minor === "number" ? (
            <><dt>К выплате специалисту провайдером</dt><dd>{money(status.beneficiary_amount_minor, status.currency)}</dd></>
          ) : null}
          <dt>Расчёт</dt><dd>{SETTLEMENT_LABELS[status.settlement_status]}</dd>
          <dt>Выплата</dt><dd>{PAYOUT_LABELS[status.payout_status]}</dd>
          <dt>Сверка</dt><dd>{RECONCILIATION_LABELS[status.reconciliation_outcome]}</dd>
          {status.disputed_amount_minor > 0 ? (
            <><dt>Оспорено через провайдера</dt><dd>{money(status.disputed_amount_minor, status.currency)}</dd></>
          ) : null}
        </dl>
      ) : null}
    </section>
  );
}
