import React, {useEffect, useRef, useState} from "react";
import {Pressable, Text, TextInput, View} from "react-native";

import {
  discrepancyLabels,
  formatMinor,
  payoutLabels,
  readOwnedFinanceStatus,
  reconciliationLabels,
  settlementLabels,
} from "./mobile-finance-status-client.ts";
import type {FinanceOrderStatus} from "../../../packages/contracts/src/generated/apgic-v1.ts";

const validOrderID = /^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$/;

type Props = {baseURL: string; sessionCookie?: string; enabled: boolean; initialOrderID?: string};

const errorText: Record<string, string> = {
  FINANCE_SESSION_REQUIRED: "Войдите, чтобы увидеть финансовый статус заказа.",
  FINANCE_ORDER_NOT_FOUND: "Заказ не найден или недоступен для вашей учётной записи.",
};

/**
 * iOS/Android owner view of R4 provider-executed settlement, payout and
 * reconciliation. Status is always re-read from APGIC; nothing is cached on
 * the device and no APGIC-held balance exists to display.
 */
export default function NativeFinanceStatus({baseURL, sessionCookie, enabled, initialOrderID}: Props) {
  const [inputID, setInputID] = useState("");
  const [status, setStatus] = useState<FinanceOrderStatus | null>(null);
  const [state, setState] = useState<"IDLE" | "LOADING" | "READY" | "ERROR">("IDLE");
  const [error, setError] = useState("");
  const epoch = useRef(0);

  async function read(requestedOrderID = inputID.trim()) {
    const orderID = requestedOrderID.trim();
    if (!validOrderID.test(orderID)) {
      setState("ERROR"); setStatus(null); setError("Укажите идентификатор заказа.");
      return;
    }
    const current = ++epoch.current;
    setState("LOADING"); setStatus(null); setError("");
    try {
      const fresh = await readOwnedFinanceStatus({baseURL, sessionCookie}, orderID);
      if (current !== epoch.current) return;
      setStatus(fresh); setState("READY");
    } catch (cause) {
      if (current !== epoch.current) return;
      const code = cause instanceof Error ? cause.message : "";
      setState("ERROR");
      setError(errorText[code] ?? "Финансовый статус временно недоступен. Попробуйте позже.");
    }
  }

  useEffect(() => {
    if (!enabled || !initialOrderID) return;
    const orderID = initialOrderID.trim();
    if (!validOrderID.test(orderID)) return;
    setInputID(orderID);
    void read(orderID);
  }, [initialOrderID, baseURL, sessionCookie, enabled]);

  if (!enabled) return null;
  return (
    <View accessibilityLabel="native-finance-status">
      <Text accessibilityRole="header">Финансовый статус заказа</Text>
      <Text>Деньги проводит и выплачивает платёжный провайдер; APGIC не хранит средства.</Text>
      <TextInput
        accessibilityLabel="native-finance-order-id"
        autoCapitalize="none"
        autoCorrect={false}
        value={inputID}
        onChangeText={setInputID}
        placeholder="Идентификатор заказа"
      />
      <Pressable accessibilityRole="button" accessibilityLabel="native-finance-refresh" onPress={() => void read()}>
        <Text>{state === "LOADING" ? "Загружаем…" : "Показать статус"}</Text>
      </Pressable>
      {state === "ERROR" ? <Text accessibilityRole="alert" accessibilityLabel="native-finance-error">{error}</Text> : null}
      {state === "READY" && status ? (
        <View accessibilityLabel={`native-finance-state:${status.settlement_status}:${status.payout_status}:${status.discrepancy_status}`}>
          <Text>Сумма заказа: {formatMinor(status.total_amount_minor, status.currency)}</Text>
          {status.platform_fee_minor !== undefined ? (
            <Text>Комиссия платформы: {formatMinor(status.platform_fee_minor, status.currency)}</Text>
          ) : null}
          {status.beneficiary_amount_minor !== undefined ? (
            <Text>К выплате специалисту провайдером: {formatMinor(status.beneficiary_amount_minor, status.currency)}</Text>
          ) : null}
          <Text>Расчёт: {settlementLabels[status.settlement_status]}</Text>
          <Text>Выплата: {payoutLabels[status.payout_status]}</Text>
          <Text>Сверка: {reconciliationLabels[status.reconciliation_outcome]}</Text>
          {status.discrepancy_status !== "NONE" ? <Text>Расхождение: {discrepancyLabels[status.discrepancy_status]}</Text> : null}
          {status.disputed_amount_minor > 0 ? (
            <Text>Оспорено через провайдера: {formatMinor(status.disputed_amount_minor, status.currency)}</Text>
          ) : null}
        </View>
      ) : null}
    </View>
  );
}
