import React, {useEffect, useRef, useState} from "react";
import {Linking, Pressable, Text, TextInput, View} from "react-native";

import type {BookingClientConfig} from "./mobile-booking-client.ts";
import {
  confirmConsultationReadiness,
  readConsultationReadiness,
  readProviderConsultationResult,
  requestConsultationJoin,
  type ConsultationReadiness,
  type ConsultationResult,
} from "./consultation-journey-client.ts";

const validBookingID = /^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$/;
// The idempotency key is a correlation reference, NOT an authorization
// credential. Permission and expiry are checked on every APGIC request.
// Use a process-scoped monotonic sequence plus wall-clock label to avoid
// hard dependencies on uninstalled React Native crypto polyfills.
let nativeJoinSequence = 0;
function createNativeJoinKey(bookingID: string, role: "CLIENT" | "SPECIALIST"): string {
  nativeJoinSequence += 1;
  return `native-room-join:${bookingID}:${role}:${Date.now().toString(36)}:${nativeJoinSequence}`;
}

type Props = {
  baseURL: string;
  sessionCookie?: string;
  enabled: boolean;
};

/**
 * iOS/Android user journey through APGIC-owned authorization to a replaceable
 * HTTPS media room. It does not fake device/media presence or business state.
 * LiveKit, SFU/TURN, reconnection and track events are owned by the provider
 * browser session and its signed webhook, not the React Native component.
 */
export default function NativeConsultationAccess({baseURL, sessionCookie, enabled}: Props) {
  const [inputID, setInputID] = useState("");
  const [bookingID, setBookingID] = useState("");
  const [readiness, setReadiness] = useState<ConsultationReadiness | null>(null);
  const [verified, setVerified] = useState<ConsultationResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");
  const pendingKey = useRef<string | null>(null);
  const config: BookingClientConfig = {baseURL, sessionCookie};

  useEffect(() => {
    setReadiness(null);
    setVerified(null);
    setStatus("");
    pendingKey.current = null;
    if (!bookingID || !enabled) return;
    let active = true;
    const refresh = async () => {
      try {
        const ready = await readConsultationReadiness(config, bookingID);
        if (!active) return;
        setReadiness(ready);
        // Never synthesize completed from the booking's state or a timeout.
        try {
          const result = await readProviderConsultationResult(config, bookingID);
          if (active) setVerified(result);
        } catch {
          if (active) setVerified(null);
        }
      } catch (error) {
        if (active) {
          setReadiness(null);
          setVerified(null);
          setStatus(error instanceof Error && error.message === "CONSULT_SESSION_REQUIRED"
            ? "Сначала войдите в APGIC как участник записи."
            : "Не удалось проверить запись. Повторите запрос.");
        }
      }
    };
    void refresh();
    const timer = setInterval(() => {void refresh();}, 12000);
    return () => {active = false; clearInterval(timer);};
    // Cookie config is owned by the caller's canonical logged-in session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bookingID, baseURL, sessionCookie, enabled]);

  function openBooking() {
    const id = inputID.trim();
    if (!validBookingID.test(id)) {
      setStatus("Введите корректный идентификатор подтверждённой записи.");
      return;
    }
    setStatus("");
    setBookingID(id);
  }

  async function becomeReady() {
    if (!bookingID || !enabled || busy) return;
    setBusy(true); setStatus("");
    try {
      const value = await confirmConsultationReadiness(config, bookingID);
      setReadiness(value);
      setStatus("Готовность подтверждена сервером. Ожидаем второго участника.");
    } catch {
      setStatus("Готовность не подтверждена. Проверьте статус и время записи.");
    } finally {
      setBusy(false);
    }
  }

  async function join() {
    if (!bookingID || !enabled || busy || !readiness ||
      readiness.booking_state !== "CONFIRMED" ||
      !readiness.client_ready_at || !readiness.specialist_ready_at ||
      verified?.state === "COMPLETED" || verified?.state === "TECHNICAL_FAILURE" ||
      readiness.consultation_state === "COMPLETED" || readiness.consultation_state === "TECHNICAL_FAILURE") return;
    setBusy(true); setStatus("");
    try {
      const key = pendingKey.current ?? createNativeJoinKey(bookingID, readiness.role);
      pendingKey.current = key;
      const response = await requestConsultationJoin(config, bookingID, key);
      // A provider-authorized short-lived token belongs only to its HTTPS
      // media origin. The room is not written to local storage or analytics.
      await Linking.openURL(response.join_uri);
      pendingKey.current = null;
      setStatus("Комната открыта. Подключение и завершение подтвердит медиапровайдер.");
    } catch {
      // Preserve the same key across network ambiguity and OS browser failure
      // so a retry cannot multiply provider grants or charge the client.
      setStatus("Комната недоступна или токен не выдан. Повторная оплата не требуется.");
    } finally {
      setBusy(false);
    }
  }

  const ended = verified?.state === "COMPLETED" &&
    typeof verified.completion_evidence_ref === "string" &&
    verified.completion_evidence_ref.length > 0;
  const terminal = ended || verified?.state === "TECHNICAL_FAILURE" ||
    readiness?.booking_state === "CANCELLED" || readiness?.booking_state === "NO_SHOW" ||
    readiness?.consultation_state === "TECHNICAL_FAILURE" || readiness?.consultation_state === "COMPLETED";
  const bothReady = Boolean(readiness?.client_ready_at && readiness?.specialist_ready_at);
  return <View accessibilityRole="summary" accessibilityLabel="native-consultation-journey" style={{gap: 12, marginTop: 16}}>
    <Text style={{fontSize: 20, fontWeight: "700"}}>Моя консультация</Text>
    <Text>Подключение происходит по подтверждённой записи и действующему праву доступа. Реальная связь открывается в защищённой комнате медиапровайдера.</Text>
    <TextInput
      accessibilityLabel="Идентификатор моей консультации"
      value={inputID}
      onChangeText={setInputID}
      placeholder="Идентификатор записи (UUID)"
      autoCapitalize="none"
      autoCorrect={false}
      style={{borderWidth: 1, borderRadius: 8, padding: 12}}
    />
    <Pressable accessibilityRole="button" accessibilityLabel="Проверить мою запись"
      disabled={!enabled || busy} onPress={openBooking}
      style={{padding: 12, borderWidth: 1, borderRadius: 8}}>
      <Text>Проверить запись</Text>
    </Pressable>
    {!enabled ? <Text accessibilityRole="alert">Консультации отключены политикой приложения или требуется обновление.</Text> : null}
    {readiness ? <View style={{gap: 8}}>
      <Text accessibilityLabel="native-consultation-booking-state">Запись: {readiness.booking_state}</Text>
      <Text>Клиент: {readiness.client_ready_at ? "готов" : "ожидается"}; специалист: {readiness.specialist_ready_at ? "готов" : "ожидается"}.</Text>
      {verified ? <Text accessibilityLabel="native-consultation-provider-state">Медиасеанс: {verified.state}</Text> : null}
      {ended ? <Text accessibilityLabel="native-consultation-provider-completed">Медиапровайдер подтвердил завершение. Результат сохранён в APGIC.</Text> : null}
      {verified?.state === "TECHNICAL_FAILURE" ?
        <Text accessibilityRole="alert">Технический сбой подтверждён. Для переноса или возврата используйте владельца записи; повторная оплата автоматически не требуется.</Text> : null}
      {!terminal && readiness.booking_state === "CONFIRMED" ? <>
        <Pressable accessibilityRole="button" accessibilityLabel="Я готов к консультации"
          disabled={busy} onPress={() => void becomeReady()}
          style={{padding: 12, borderWidth: 1, borderRadius: 8}}>
          <Text>Я готов</Text>
        </Pressable>
        <Pressable accessibilityRole="button" accessibilityLabel="Войти в защищённую комнату"
          disabled={busy || !bothReady} onPress={() => void join()}
          style={{padding: 12, borderWidth: 1, borderRadius: 8}}>
          <Text>Войти в консультацию</Text>
        </Pressable>
        {!bothReady ? <Text>Система ожидает готовность обеих сторон. Данные обновляются автоматически.</Text> : null}
      </> : null}
    </View> : null}
    {status ? <Text accessibilityRole="alert" accessibilityLabel="native-consultation-status">{status}</Text> : null}
  </View>;
}
