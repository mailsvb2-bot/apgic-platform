import React, {useCallback, useEffect, useState} from "react";
import { Linking, SafeAreaView, StyleSheet, Text, View } from "react-native";

import {
  decideCapability,
  type CapabilityState,
  type DeviceCapability,
} from "./device-capability.ts";
import {
  runInstallationE2ELifecycle,
  type MobileInstallationPlatform,
} from "./mobile-installation-client.ts";
import {
  canonicalAPGICOrigin,
  resolveCanonicalUniversalLink,
} from "./mobile-deep-link-client.ts";
import { resolvePushNotification } from "./mobile-notification-client.ts";
import {
  createOfflineCheckoutQueueItem,
  loadOfflineCheckout,
  persistOfflineCheckout,
  syncOfflineCheckout,
  type OfflineCheckoutState,
} from "./mobile-offline-checkout.ts";
import {offlineMutationStorage} from "./offline-mutation-storage.ts";
import {runNativeRealtimeE2E, type NativeRealtimeE2EResult} from "./r3-realtime-e2e.ts";

type AppProps = {
  deviceCapability?: DeviceCapability;
  deviceCapabilityState?: CapabilityState;
  installationE2EBaseURL?: string;
  installationE2ESessionCookie?: string;
  installationE2EInstallationID?: string;
  installationE2EPlatform?: MobileInstallationPlatform;
  deepLinkAPIBaseURL?: string;
  deepLinkE2ESessionCookie?: string;
  deepLinkE2EURL?: string;
  notificationE2EBaseURL?: string;
  notificationE2ESessionCookie?: string;
  notificationE2EDeliveryID?: string;
  notificationE2EIntentID?: string;
  offlineMutationE2EBaseURL?: string;
  offlineMutationE2ESessionCookie?: string;
  offlineMutationE2EHoldID?: string;
  offlineMutationE2EIdempotencyKey?: string;
  offlineMutationE2EMethodCode?: string;
  realtimeE2EEvents?: string;
  realtimeE2EReconnectFailures?: string;
};

const fallbackCopy = {
  PERMISSION_DENIED: "Доступ запрещён. Основной сценарий не изменяет данные и предлагает безопасный обход.",
  OS_RESTRICTED: "Доступ ограничен системой. Основной сценарий остаётся доступен без искажения server truth.",
  CAPABILITY_UNAVAILABLE: "Возможность недоступна на устройстве. Используется безопасный fallback.",
} as const;

const actionCopy = {
  CONTINUE: "Возможность доступна.",
  REQUEST_PERMISSION: "Нужно запросить разрешение операционной системы.",
  REFRESH_STATE: "Состояние нужно перечитать у операционной системы.",
  FALLBACK: "Используется безопасный fallback.",
} as const;

export default function App({
  deviceCapability = "MICROPHONE",
  deviceCapabilityState = "UNKNOWN",
  installationE2EBaseURL,
  installationE2ESessionCookie,
  installationE2EInstallationID,
  installationE2EPlatform,
  deepLinkAPIBaseURL = canonicalAPGICOrigin,
  deepLinkE2ESessionCookie,
  deepLinkE2EURL,
  notificationE2EBaseURL,
  notificationE2ESessionCookie,
  notificationE2EDeliveryID,
  notificationE2EIntentID,
  offlineMutationE2EBaseURL,
  offlineMutationE2ESessionCookie,
  offlineMutationE2EHoldID,
  offlineMutationE2EIdempotencyKey,
  offlineMutationE2EMethodCode,
  realtimeE2EEvents,
  realtimeE2EReconnectFailures,
}: AppProps) {
  const decision = decideCapability(deviceCapabilityState);
  const [installationE2E, setInstallationE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {status: "PASS"; identityID: string; pushGeneration: number; state: "REVOKED"}
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [deepLinkState, setDeepLinkState] = useState<
    | {status: "IDLE" | "RESOLVING"}
    | {status: "OPEN"; target: string}
    | {status: "FALLBACK"; target: string}
    | {status: "BLOCKED"}
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [notificationE2E, setNotificationE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {status: "PASS"; intentID: string; previewMode: "GENERIC" | "FULL"}
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [offlineMutationE2E, setOfflineMutationE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {
        status: OfflineCheckoutState;
        attempts: number;
        sideEffectRef?: string;
        reasonCode?: string;
      }
    | {status: "ERROR"; reason: string}
  >({status: "IDLE"});

  const [realtimeE2E, setRealtimeE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | ({status: "PASS"} & NativeRealtimeE2EResult)
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const handleDeepLink = useCallback(
    async (url: string) => {
      setDeepLinkState({status: "RESOLVING"});
      try {
        const action = await resolveCanonicalUniversalLink(url, {
          apiOrigin: deepLinkAPIBaseURL,
          sessionCookie: deepLinkE2ESessionCookie,
        });
        if (action.action === "OPEN_APP_PATH") {
          setDeepLinkState({status: "OPEN", target: action.target});
          return;
        }
        if (action.action === "OPEN_WEB_FALLBACK") {
          setDeepLinkState({status: "FALLBACK", target: action.target});
          await Linking.openURL(action.target);
          return;
        }
        setDeepLinkState({status: "BLOCKED"});
      } catch (error: unknown) {
        setDeepLinkState({
          status: "FAIL",
          reason: error instanceof Error ? error.message : "DEEPLINK_RESOLUTION_FAILED",
        });
      }
    },
    [deepLinkAPIBaseURL, deepLinkE2ESessionCookie],
  );

  useEffect(() => {
    let active = true;
    const receive = (url: string | null | undefined) => {
      if (active && url) {
        void handleDeepLink(url);
      }
    };
    if (deepLinkE2EURL) {
      receive(deepLinkE2EURL);
    } else {
      void Linking.getInitialURL().then(receive);
    }
    const subscription = Linking.addEventListener("url", ({url}) => receive(url));
    return () => {
      active = false;
      subscription.remove();
    };
  }, [deepLinkE2EURL, handleDeepLink]);

  useEffect(() => {
    if (
      !notificationE2EBaseURL ||
      !notificationE2ESessionCookie ||
      !notificationE2EDeliveryID ||
      !notificationE2EIntentID
    ) {
      return;
    }
    let active = true;
    setNotificationE2E({status: "RUNNING"});
    void resolvePushNotification(
      {
        contract_version: "notification-transport-v1",
        delivery_id: notificationE2EDeliveryID,
        intent_id: notificationE2EIntentID,
      },
      {
        baseURL: notificationE2EBaseURL,
        sessionCookie: notificationE2ESessionCookie,
      },
    ).then(
      (projection) => {
        if (active) {
          setNotificationE2E({
            status: "PASS",
            intentID: projection.intent_id,
            previewMode: projection.preview_mode,
          });
        }
      },
      (error: unknown) => {
        if (active) {
          setNotificationE2E({
            status: "FAIL",
            reason: error instanceof Error ? error.message : "NOTIFICATION_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    notificationE2EBaseURL,
    notificationE2ESessionCookie,
    notificationE2EDeliveryID,
    notificationE2EIntentID,
  ]);

  useEffect(() => {
    if (!offlineMutationE2EBaseURL || !offlineMutationE2ESessionCookie) {
      return;
    }
    let active = true;
    setOfflineMutationE2E({status: "RUNNING"});

    void (async () => {
      try {
        let item = null;
        if (
          offlineMutationE2EHoldID &&
          offlineMutationE2EIdempotencyKey &&
          offlineMutationE2EMethodCode
        ) {
          await offlineMutationStorage.clear();
          const createdAt = new Date();
          item = createOfflineCheckoutQueueItem({
            idempotencyKey: offlineMutationE2EIdempotencyKey,
            holdID: offlineMutationE2EHoldID,
            methodCode: offlineMutationE2EMethodCode,
            now: createdAt,
            expiresAt: new Date(createdAt.getTime() + 10 * 60_000),
            maxAttempts: 3,
          });
          await persistOfflineCheckout(offlineMutationStorage, item);
        } else {
          item = await loadOfflineCheckout(offlineMutationStorage);
        }
        if (!item) {
          throw new Error("OFFLINE_MUTATION_E2E_QUEUE_MISSING");
        }
        const result = await syncOfflineCheckout(
          item,
          offlineMutationStorage,
          {
            baseURL: offlineMutationE2EBaseURL,
            sessionCookie: offlineMutationE2ESessionCookie,
          },
        );
        if (active) {
          setOfflineMutationE2E({
            status: result.state,
            attempts: result.attempts,
            sideEffectRef: result.side_effect_ref,
            reasonCode: result.reason_code,
          });
        }
      } catch (error: unknown) {
        if (active) {
          setOfflineMutationE2E({
            status: "ERROR",
            reason:
              error instanceof Error
                ? error.message
                : "OFFLINE_MUTATION_E2E_FAILED",
          });
        }
      }
    })();

    return () => {
      active = false;
    };
  }, [
    offlineMutationE2EBaseURL,
    offlineMutationE2ESessionCookie,
    offlineMutationE2EHoldID,
    offlineMutationE2EIdempotencyKey,
    offlineMutationE2EMethodCode,
  ]);

  useEffect(() => {
    if (!realtimeE2EEvents) {
      return;
    }
    let active = true;
    setRealtimeE2E({status: "RUNNING"});
    const reconnectFailures = Number.parseInt(realtimeE2EReconnectFailures ?? "0", 10);
    void runNativeRealtimeE2E(
      realtimeE2EEvents,
      Number.isFinite(reconnectFailures) && reconnectFailures >= 0 ? reconnectFailures : 0,
    ).then(
      (result) => {
        if (active) setRealtimeE2E({status: "PASS", ...result});
      },
      (error: unknown) => {
        if (active) {
          setRealtimeE2E({
            status: "FAIL",
            reason: error instanceof Error ? error.message : "REALTIME_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [realtimeE2EEvents, realtimeE2EReconnectFailures]);

  useEffect(() => {
    if (
      !installationE2EBaseURL ||
      !installationE2ESessionCookie ||
      !installationE2EInstallationID ||
      !installationE2EPlatform
    ) {
      return;
    }
    let active = true;
    setInstallationE2E({status: "RUNNING"});
    void runInstallationE2ELifecycle({
      baseURL: installationE2EBaseURL,
      sessionCookie: installationE2ESessionCookie,
      installationID: installationE2EInstallationID,
      platform: installationE2EPlatform,
    }).then(
      (result) => {
        if (active) {
          setInstallationE2E({status: "PASS", ...result});
        }
      },
      (error: unknown) => {
        if (active) {
          setInstallationE2E({
            status: "FAIL",
            reason: error instanceof Error ? error.message : "MOBILE_INSTALLATION_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    installationE2EBaseURL,
    installationE2ESessionCookie,
    installationE2EInstallationID,
    installationE2EPlatform,
  ]);

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.card} accessibilityRole="summary">
        <Text style={styles.eyebrow}>R0 · Foundation</Text>
        <Text style={styles.title}>APGIC</Text>
        <Text style={styles.body}>
          Одна Identity и одна server truth для iOS, Android, Web и PWA.
        </Text>

        <View
          style={styles.capability}
          accessibilityLabel={`capability:${deviceCapability}`}
        >
          <Text style={styles.capabilityTitle}>Готовность устройства</Text>
          <Text accessibilityLabel={`capability-state:${decision.state}`}>
            {deviceCapability}: {decision.state}
          </Text>
          <Text accessibilityLabel={`capability-action:${decision.action}`}>
            {actionCopy[decision.action]}
          </Text>
          {decision.action === "FALLBACK" ? (
            <Text
              accessibilityLabel={`capability-fallback:${decision.reason}`}
            >
              {fallbackCopy[decision.reason]}
            </Text>
          ) : null}
        </View>

        {deepLinkState.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`deep-link-state:${deepLinkState.status}`}>
              Deep link: {deepLinkState.status}
            </Text>
            {deepLinkState.status === "OPEN" ? (
              <Text accessibilityLabel={`deep-link-target:${deepLinkState.target}`}>
                {deepLinkState.target}
              </Text>
            ) : null}
            {deepLinkState.status === "FALLBACK" ? (
              <Text accessibilityLabel={`deep-link-fallback:${deepLinkState.target}`}>
                Web fallback
              </Text>
            ) : null}
          </View>
        ) : null}

        {notificationE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`notification-e2e:${notificationE2E.status}`}>
              Notification E2E: {notificationE2E.status}
            </Text>
            {notificationE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`notification-e2e-intent:${notificationE2E.intentID}`}>
                  Intent: {notificationE2E.intentID}
                </Text>
                <Text accessibilityLabel={`notification-e2e-preview:${notificationE2E.previewMode}`}>
                  Preview: {notificationE2E.previewMode}
                </Text>
              </>
            ) : null}
            {notificationE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`notification-e2e-error:${notificationE2E.reason}`}>
                Notification transport failed safely.
              </Text>
            ) : null}
          </View>
        ) : null}

        {offlineMutationE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text
              accessibilityLabel={`offline-mutation-e2e:${offlineMutationE2E.status}`}
            >
              Offline mutation: {offlineMutationE2E.status}
            </Text>
            {"attempts" in offlineMutationE2E ? (
              <Text
                accessibilityLabel={`offline-mutation-attempts:${offlineMutationE2E.attempts}`}
              >
                Attempts: {offlineMutationE2E.attempts}
              </Text>
            ) : null}
            {"sideEffectRef" in offlineMutationE2E &&
            offlineMutationE2E.sideEffectRef ? (
              <Text
                accessibilityLabel={`offline-mutation-side-effect:${offlineMutationE2E.sideEffectRef}`}
              >
                Server side effect confirmed.
              </Text>
            ) : null}
            {"reasonCode" in offlineMutationE2E &&
            offlineMutationE2E.reasonCode ? (
              <Text
                accessibilityLabel={`offline-mutation-reason:${offlineMutationE2E.reasonCode}`}
              >
                {offlineMutationE2E.reasonCode}
              </Text>
            ) : null}
            {offlineMutationE2E.status === "ERROR" ? (
              <Text
                accessibilityLabel={`offline-mutation-error:${offlineMutationE2E.reason}`}
              >
                Offline mutation runtime failed safely.
              </Text>
            ) : null}
          </View>
        ) : null}

        {realtimeE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`realtime-e2e:${realtimeE2E.status}`}>
              Realtime E2E: {realtimeE2E.status}
            </Text>
            {realtimeE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`realtime-phase:${realtimeE2E.phase}`}>Phase: {realtimeE2E.phase}</Text>
                <Text accessibilityLabel={`realtime-business-transition:${realtimeE2E.businessTransitions.every((value) => value === "NONE") ? "NONE" : "INVALID"}`}>Business transition: NONE</Text>
                <Text accessibilityLabel={`realtime-audio-route:${realtimeE2E.audioRoute}`}>Audio route: {realtimeE2E.audioRoute}</Text>
                <Text accessibilityLabel={`realtime-app-state:${realtimeE2E.appState}`}>App state: {realtimeE2E.appState}</Text>
                <Text accessibilityLabel={`realtime-network-state:${realtimeE2E.networkState}`}>Network: {realtimeE2E.networkState}</Text>
                <Text accessibilityLabel={`realtime-provider-actions:${realtimeE2E.providerActions.join("|")}`}>Provider actions recorded.</Text>
              </>
            ) : null}
            {realtimeE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`realtime-e2e-error:${realtimeE2E.reason}`}>Realtime lifecycle failed safely.</Text>
            ) : null}
          </View>
        ) : null}

        {installationE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`installation-e2e:${installationE2E.status}`}>
              Installation E2E: {installationE2E.status}
            </Text>
            {installationE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`installation-e2e-state:${installationE2E.state}`}>
                  State: {installationE2E.state}
                </Text>
                <Text accessibilityLabel={`installation-e2e-generation:${installationE2E.pushGeneration}`}>
                  Push generation: {installationE2E.pushGeneration}
                </Text>
              </>
            ) : null}
            {installationE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`installation-e2e-error:${installationE2E.reason}`}>
                Installation lifecycle failed safely.
              </Text>
            ) : null}
          </View>
        ) : null}
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, justifyContent: "center", padding: 24 },
  card: { gap: 12 },
  eyebrow: { fontWeight: "700", textTransform: "uppercase", letterSpacing: 1.2 },
  title: { fontWeight: "800", fontSize: 48 },
  body: { fontSize: 18, lineHeight: 27 },
  capability: { gap: 8, marginTop: 12 },
  capabilityTitle: { fontSize: 20, fontWeight: "700" },
});