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
