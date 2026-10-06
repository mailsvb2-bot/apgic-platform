import React, {useCallback, useEffect, useState} from "react";
import {
  AccessibilityInfo,
  Linking,
  PixelRatio,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import {apiContractVersion} from "./api-contract.ts";
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
import {
  fetchClientCompatibility,
} from "./mobile-compatibility-client.ts";
import type {MobilePlatform} from "../../../packages/contracts/src/mobile-policy.ts";
import {resolveRemoteConfig} from "./remote-config-client.ts";
import {remoteConfigStorage} from "./remote-config-storage.ts";
import { resolvePushNotification } from "./mobile-notification-client.ts";
import {
  createOfflineCheckoutQueueItem,
  loadOfflineCheckout,
  persistOfflineCheckout,
  syncOfflineCheckout,
  type OfflineCheckoutState,
} from "./mobile-offline-checkout.ts";
import {offlineMutationStorage} from "./offline-mutation-storage.ts";
import {clearUserScopedLocalState, secureCredentialStorage} from "./secure-local-storage.ts";
import {runNativeRealtimeE2E, type NativeRealtimeE2EResult} from "./r3-realtime-e2e.ts";
import type {HelpIntent} from "../../../packages/contracts/src/generated/apgic-v1.ts";
import {
  confirmMobileHelpIntent,
  createMobileHelpIntent,
  runMobileDemandE2E,
  type MobileDemandE2EResult,
} from "./mobile-demand-client.ts";
import {
  runWorkspaceE2EFlow,
  type WorkspaceE2EResult,
} from "./mobile-workspace-client.ts";
import {
  runNativeDeletionE2E,
  type NativeDeletionE2EResult,
  type NativeSurface,
} from "./r1-cross-surface.ts";

type AppProps = {
  deviceCapability?: DeviceCapability;
  deviceCapabilityState?: CapabilityState;
  installationE2EBaseURL?: string;
  installationE2ESessionCookie?: string;
  installationE2EInstallationID?: string;
  installationE2EPlatform?: MobileInstallationPlatform;
  workspaceE2EBaseURL?: string;
  workspaceE2ESessionCookie?: string;
  deletionE2EBaseURL?: string;
  deletionE2ESessionCookie?: string;
  deletionE2EIdentityID?: string;
  deletionE2ERequestID?: string;
  deletionE2EPlatform?: NativeSurface;
  demandAPIBaseURL?: string;
  demandE2EBaseURL?: string;
  demandE2ESessionCookie?: string;
  demandE2EFreeText?: string;
  demandE2ECorrectedTopics?: string;
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
  realtimeE2EConsultationID?: string;
  compatibilityBaseURL?: string;
  remoteConfigBaseURL?: string;
  remoteConfigTrustedKeyID?: string;
  remoteConfigTrustedPublicKeyBase64?: string;
  compatibilityPlatform?: MobilePlatform;
  appVersion?: string;
  buildNumber?: string;
  compatibilityContractVersion?: string;
  accessibilityE2EEnabled?: boolean;
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
  workspaceE2EBaseURL,
  workspaceE2ESessionCookie,
  deletionE2EBaseURL,
  deletionE2ESessionCookie,
  deletionE2EIdentityID,
  deletionE2ERequestID,
  deletionE2EPlatform,
  demandAPIBaseURL = canonicalAPGICOrigin,
  demandE2EBaseURL,
  demandE2ESessionCookie,
  demandE2EFreeText,
  demandE2ECorrectedTopics,
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
  realtimeE2EConsultationID,
  compatibilityBaseURL = canonicalAPGICOrigin,
  remoteConfigBaseURL,
  remoteConfigTrustedKeyID,
  remoteConfigTrustedPublicKeyBase64,
  compatibilityPlatform,
  appVersion,
  buildNumber,
  compatibilityContractVersion = apiContractVersion,
  accessibilityE2EEnabled = false,
}: AppProps) {
  const decision = decideCapability(deviceCapabilityState);
  const [compatibility, setCompatibility] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {
        status: "PASS";
        decision: Awaited<ReturnType<typeof fetchClientCompatibility>>;
      }
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [remoteConfig, setRemoteConfig] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {
        status: "PASS";
        source: Awaited<ReturnType<typeof resolveRemoteConfig>>["source"];
        disabledCapabilities: Awaited<ReturnType<typeof resolveRemoteConfig>>["disabledCapabilities"];
        reasonCode: string;
      }
  >({status: "IDLE"});

  const [installationE2E, setInstallationE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | {status: "PASS"; identityID: string; pushGeneration: number; state: "REVOKED"}
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [workspaceE2E, setWorkspaceE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | ({status: "PASS"} & WorkspaceE2EResult)
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [deletionE2E, setDeletionE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | ({status: "PASS"} & NativeDeletionE2EResult)
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  const [demandText, setDemandText] = useState("");
  const [demandIntent, setDemandIntent] = useState<HelpIntent | null>(null);
  const [demandTopics, setDemandTopics] = useState<string[]>([]);
  const [demandCustomTopic, setDemandCustomTopic] = useState("");
  const [demandStatus, setDemandStatus] = useState<
    "IDLE" | "RUNNING" | "DRAFT" | "CONFIRMED" | "FAIL"
  >("IDLE");
  const [demandError, setDemandError] = useState("");
  const [demandE2E, setDemandE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | ({status: "PASS"} & MobileDemandE2EResult)
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

  const [reducedMotionEnabled, setReducedMotionEnabled] = useState(false);
  const fontScale = PixelRatio.getFontScale();

  const [realtimeE2E, setRealtimeE2E] = useState<
    | {status: "IDLE" | "RUNNING"}
    | ({status: "PASS"} & NativeRealtimeE2EResult)
    | {status: "FAIL"; reason: string}
  >({status: "IDLE"});

  useEffect(() => {
    let active = true;
    void AccessibilityInfo.isReduceMotionEnabled().then((enabled) => {
      if (active) {
        setReducedMotionEnabled(enabled);
      }
    });
    const subscription = AccessibilityInfo.addEventListener(
      "reduceMotionChanged",
      (enabled) => setReducedMotionEnabled(enabled),
    );
    return () => {
      active = false;
      subscription.remove();
    };
  }, []);

  const compatibilityRequired = Boolean(
    compatibilityPlatform && appVersion && buildNumber,
  );
  const compatibilityAllowsRuntime =
    !compatibilityRequired ||
    (compatibility.status === "PASS" &&
      compatibility.decision.status !== "UPDATE_REQUIRED");
  const showCompatibilityNotice =
    compatibility.status !== "IDLE" &&
    (compatibility.status !== "PASS" ||
      compatibility.decision.status !== "SUPPORTED");
  const remoteConfigRequired = Boolean(remoteConfigBaseURL);
  const remoteConfigAllowsRealtime =
    !remoteConfigRequired ||
    (remoteConfig.status === "PASS" &&
      !remoteConfig.disabledCapabilities.includes("REALTIME_CONSULTATION"));

  // Installed-app E2E probes exercise isolated native capabilities. Keep the
  // production journey out of debug-only evidence layouts so proof labels do
  // not depend on emulator viewport scrolling. Production HelpIntent remains a
  // real ScrollView-backed user journey outside E2E mode.
  const demandE2EActive = Boolean(
    demandE2EBaseURL &&
      demandE2ESessionCookie &&
      demandE2EFreeText &&
      demandE2ECorrectedTopics,
  );
  const infrastructureE2EActive = Boolean(
    installationE2EBaseURL ||
      workspaceE2EBaseURL ||
      deletionE2EBaseURL ||
      demandE2EActive ||
      deepLinkE2EURL ||
      notificationE2EBaseURL ||
      offlineMutationE2EBaseURL ||
      realtimeE2EEvents ||
      accessibilityE2EEnabled ||
      (compatibilityBaseURL !== canonicalAPGICOrigin && !demandE2EBaseURL) ||
      (remoteConfigBaseURL && remoteConfigBaseURL !== canonicalAPGICOrigin),
  );

  const analyzeDemand = useCallback(async () => {
    if (!compatibilityAllowsRuntime) {
      setDemandError("Сначала завершите проверку совместимости или обновите приложение.");
      return;
    }
    const freeText = demandText.trim();
    if (!freeText) {
      setDemandError("Опишите, с чем нужна помощь.");
      return;
    }
    setDemandError("");
    setDemandStatus("RUNNING");
    try {
      const intent = await createMobileHelpIntent(freeText, {
        baseURL: demandAPIBaseURL,
        sessionCookie: demandE2ESessionCookie,
      });
      setDemandIntent(intent);
      setDemandTopics([...intent.topics]);
      setDemandStatus("DRAFT");
    } catch (error: unknown) {
      setDemandStatus("FAIL");
      setDemandError(
        error instanceof Error ? error.message : "Не удалось разобрать запрос.",
      );
    }
  }, [compatibilityAllowsRuntime, demandAPIBaseURL, demandE2ESessionCookie, demandText]);

  const toggleDemandTopic = useCallback((topic: string) => {
    setDemandTopics((current) =>
      current.includes(topic)
        ? current.filter((value) => value !== topic)
        : [...current, topic],
    );
  }, []);

  const addDemandTopic = useCallback(() => {
    const topic = demandCustomTopic.trim();
    if (!topic) return;
    setDemandTopics((current) => current.includes(topic) ? current : [...current, topic]);
    setDemandCustomTopic("");
  }, [demandCustomTopic]);

  const confirmDemand = useCallback(async () => {
    if (!compatibilityAllowsRuntime) {
      setDemandError("Сначала завершите проверку совместимости или обновите приложение.");
      return;
    }
    if (!demandIntent || demandTopics.length === 0) {
      setDemandError("Оставьте хотя бы одну подходящую тему.");
      return;
    }
    setDemandError("");
    setDemandStatus("RUNNING");
    try {
      const confirmed = await confirmMobileHelpIntent(
        demandIntent.id,
        {
          topics: demandTopics,
          goals: demandIntent.goals,
          context: demandIntent.context,
        },
        {
          baseURL: demandAPIBaseURL,
          sessionCookie: demandE2ESessionCookie,
        },
      );
      setDemandIntent(confirmed);
      setDemandTopics([...confirmed.topics]);
      setDemandStatus("CONFIRMED");
    } catch (error: unknown) {
      setDemandStatus("FAIL");
      setDemandError(
        error instanceof Error ? error.message : "Не удалось подтвердить запрос.",
      );
    }
  }, [compatibilityAllowsRuntime, demandAPIBaseURL, demandE2ESessionCookie, demandIntent, demandTopics]);

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
    if (!compatibilityBaseURL || !compatibilityPlatform || !appVersion || !buildNumber) {
      return;
    }
    let active = true;
    setCompatibility({status: "RUNNING"});
    void fetchClientCompatibility({
      baseURL: compatibilityBaseURL,
      platform: compatibilityPlatform,
      appVersion,
      buildNumber,
      contractVersion: compatibilityContractVersion,
    }).then(
      (decision) => {
        if (active) {
          setCompatibility({status: "PASS", decision});
        }
      },
      (error: unknown) => {
        if (active) {
          setCompatibility({
            status: "FAIL",
            reason:
              error instanceof Error
                ? error.message
                : "CLIENT_COMPATIBILITY_CHECK_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    compatibilityBaseURL,
    compatibilityPlatform,
    appVersion,
    buildNumber,
    compatibilityContractVersion,
  ]);

  useEffect(() => {
    if (!remoteConfigBaseURL) {
      return;
    }
    let active = true;
    setRemoteConfig({status: "RUNNING"});
    const trustedKeys =
      remoteConfigTrustedKeyID && remoteConfigTrustedPublicKeyBase64
        ? {[remoteConfigTrustedKeyID]: remoteConfigTrustedPublicKeyBase64}
        : {};
    void resolveRemoteConfig({
      baseURL: remoteConfigBaseURL,
      storage: remoteConfigStorage,
      trustedKeys,
    }).then((result) => {
      if (active) {
        setRemoteConfig({
          status: "PASS",
          source: result.source,
          disabledCapabilities: result.disabledCapabilities,
          reasonCode: result.reasonCode,
        });
      }
    });
    return () => {
      active = false;
    };
  }, [
    remoteConfigBaseURL,
    remoteConfigTrustedKeyID,
    remoteConfigTrustedPublicKeyBase64,
  ]);

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
      !compatibilityAllowsRuntime ||
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
    compatibilityAllowsRuntime,
    notificationE2EBaseURL,
    notificationE2ESessionCookie,
    notificationE2EDeliveryID,
    notificationE2EIntentID,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !offlineMutationE2EBaseURL ||
      !offlineMutationE2ESessionCookie
    ) {
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
    compatibilityAllowsRuntime,
    offlineMutationE2EBaseURL,
    offlineMutationE2ESessionCookie,
    offlineMutationE2EHoldID,
    offlineMutationE2EIdempotencyKey,
    offlineMutationE2EMethodCode,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !remoteConfigAllowsRealtime ||
      !realtimeE2EEvents
    ) {
      return;
    }
    let active = true;
    setRealtimeE2E({status: "RUNNING"});
    const reconnectFailures = Number.parseInt(realtimeE2EReconnectFailures ?? "0", 10);
    void runNativeRealtimeE2E(
      realtimeE2EEvents,
      Number.isFinite(reconnectFailures) && reconnectFailures >= 0 ? reconnectFailures : 0,
      realtimeE2EConsultationID || "consultation-native-e2e",
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
  }, [
    compatibilityAllowsRuntime,
    remoteConfigAllowsRealtime,
    realtimeE2EEvents,
    realtimeE2EReconnectFailures,
    realtimeE2EConsultationID,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !installationE2EBaseURL ||
      !installationE2ESessionCookie ||
      !installationE2EInstallationID ||
      !installationE2EPlatform
    ) {
      return;
    }
    let active = true;
    setInstallationE2E({status: "RUNNING"});
    void runInstallationE2ELifecycle(
      {
        baseURL: installationE2EBaseURL,
        sessionCookie: installationE2ESessionCookie,
        installationID: installationE2EInstallationID,
        platform: installationE2EPlatform,
      },
      undefined,
      {
        saveCredential: secureCredentialStorage.save,
        loadCredential: secureCredentialStorage.load,
        clearUserScopedState: clearUserScopedLocalState,
      },
    ).then(
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
    compatibilityAllowsRuntime,
    installationE2EBaseURL,
    installationE2ESessionCookie,
    installationE2EInstallationID,
    installationE2EPlatform,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !workspaceE2EBaseURL ||
      !workspaceE2ESessionCookie
    ) {
      return;
    }
    let active = true;
    setWorkspaceE2E({status: "RUNNING"});
    void runWorkspaceE2EFlow({
      baseURL: workspaceE2EBaseURL,
      sessionCookie: workspaceE2ESessionCookie,
    }).then(
      (result) => {
        if (active) {
          setWorkspaceE2E({status: "PASS", ...result});
        }
      },
      (error: unknown) => {
        if (active) {
          setWorkspaceE2E({
            status: "FAIL",
            reason:
              error instanceof Error ? error.message : "MOBILE_WORKSPACE_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    compatibilityAllowsRuntime,
    workspaceE2EBaseURL,
    workspaceE2ESessionCookie,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !demandE2EActive ||
      !demandE2EBaseURL ||
      !demandE2ESessionCookie ||
      !demandE2EFreeText ||
      !demandE2ECorrectedTopics
    ) {
      return;
    }
    let active = true;
    setDemandE2E({status: "RUNNING"});
    const correctedTopics = demandE2ECorrectedTopics
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean);
    void runMobileDemandE2E({
      baseURL: demandE2EBaseURL,
      sessionCookie: demandE2ESessionCookie,
      freeText: demandE2EFreeText,
      correctedTopics,
    }).then(
      (result) => {
        if (active) setDemandE2E({status: "PASS", ...result});
      },
      (error: unknown) => {
        if (active) {
          setDemandE2E({
            status: "FAIL",
            reason:
              error instanceof Error ? error.message : "MOBILE_DEMAND_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    compatibilityAllowsRuntime,
    demandE2EActive,
    demandE2EBaseURL,
    demandE2ESessionCookie,
    demandE2EFreeText,
    demandE2ECorrectedTopics,
  ]);

  useEffect(() => {
    if (
      !compatibilityAllowsRuntime ||
      !deletionE2EBaseURL ||
      !deletionE2ESessionCookie ||
      !deletionE2EIdentityID ||
      !deletionE2ERequestID ||
      !deletionE2EPlatform
    ) {
      return;
    }
    let active = true;
    setDeletionE2E({status: "RUNNING"});
    void runNativeDeletionE2E({
      baseURL: deletionE2EBaseURL,
      sessionCookie: deletionE2ESessionCookie,
      identityID: deletionE2EIdentityID,
      requestID: deletionE2ERequestID,
      platform: deletionE2EPlatform,
    }).then(
      (result) => {
        if (active) setDeletionE2E({status: "PASS", ...result});
      },
      (error: unknown) => {
        if (active) {
          setDeletionE2E({
            status: "FAIL",
            reason:
              error instanceof Error ? error.message : "MOBILE_DELETION_E2E_FAILED",
          });
        }
      },
    );
    return () => {
      active = false;
    };
  }, [
    compatibilityAllowsRuntime,
    deletionE2EBaseURL,
    deletionE2ESessionCookie,
    deletionE2EIdentityID,
    deletionE2ERequestID,
    deletionE2EPlatform,
  ]);

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView
        contentContainerStyle={styles.card}
        keyboardShouldPersistTaps="handled"
        accessibilityRole="summary"
      >
        <Text style={styles.eyebrow}>R0 · Foundation</Text>
        <Text style={styles.title}>APGIC</Text>
        <Text style={styles.body}>
          Одна Identity и одна server truth для iOS, Android, Web и PWA.
        </Text>

        {!infrastructureE2EActive ? (
          <View style={styles.demandCard} accessibilityRole="summary">
            <Text style={styles.capabilityTitle}>С чем нужна помощь</Text>
            <Text style={styles.body}>
              Опишите ситуацию своими словами. APGIC предложит темы, а вы сможете исправить их перед подтверждением.
            </Text>
            <TextInput
              accessibilityLabel="С чем нужна помощь"
              multiline
              value={demandText}
              onChangeText={setDemandText}
              placeholder="Например: тревожно перед выступлениями и плохо сплю"
              style={styles.demandInput}
            />
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Разобрать запрос"
              disabled={demandStatus === "RUNNING" || !compatibilityAllowsRuntime}
              onPress={() => void analyzeDemand()}
              style={styles.demandAction}
            >
              <Text>Разобрать запрос</Text>
            </Pressable>
            {demandIntent ? (
              <View style={styles.capability}>
                <Text accessibilityLabel="native-demand-notice">{demandIntent.notice}</Text>
                <Text accessibilityLabel={`native-demand-diagnosis:${demandIntent.diagnosis_asserted}`}>
                  Это не диагноз. Диагноз поставлен: {demandIntent.diagnosis_asserted ? "да" : "нет"}.
                </Text>
                <Text>Уточните темы запроса:</Text>
                {Array.from(new Set([...demandIntent.topics, ...demandTopics])).map((topic) => {
                  const selected = demandTopics.includes(topic);
                  return (
                    <Pressable
                      key={topic}
                      accessibilityRole="checkbox"
                      accessibilityState={{checked: selected}}
                      accessibilityLabel={`Тема ${topic}`}
                      onPress={() => toggleDemandTopic(topic)}
                      style={styles.demandTopic}
                    >
                      <Text>{selected ? "✓ " : ""}{topic}</Text>
                    </Pressable>
                  );
                })}
                <TextInput
                  accessibilityLabel="Добавить свою тему запроса"
                  value={demandCustomTopic}
                  onChangeText={setDemandCustomTopic}
                  onSubmitEditing={addDemandTopic}
                  placeholder="Добавить или заменить тему"
                  style={styles.demandInput}
                />
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel="Добавить свою тему запроса"
                  disabled={!demandCustomTopic.trim()}
                  onPress={addDemandTopic}
                  style={styles.demandAction}
                >
                  <Text>Добавить тему</Text>
                </Pressable>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel="Подтвердить темы запроса"
                  disabled={demandStatus === "RUNNING" || demandTopics.length === 0 || !compatibilityAllowsRuntime}
                  onPress={() => void confirmDemand()}
                  style={styles.demandAction}
                >
                  <Text>Подтвердить темы</Text>
                </Pressable>
                {demandStatus === "CONFIRMED" ? (
                  <Text accessibilityLabel="native-demand-confirmed">
                    Запрос подтверждён пользователем.
                  </Text>
                ) : null}
              </View>
            ) : null}
            {demandError ? (
              <Text accessibilityRole="alert" accessibilityLabel="native-demand-error">
                {demandError}
              </Text>
            ) : null}
          </View>
        ) : null}

        {demandE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`demand-e2e:${demandE2E.status}`}>
              HelpIntent E2E: {demandE2E.status}
            </Text>
            {demandE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`demand-e2e-diagnosis:${demandE2E.diagnosisAsserted}`}>
                  Diagnosis asserted: {String(demandE2E.diagnosisAsserted)}
                </Text>
                <Text accessibilityLabel={`demand-e2e-correction:${demandE2E.userCorrectionObserved}`}>
                  User correction preserved.
                </Text>
                <Text accessibilityLabel={`demand-e2e-topics:${demandE2E.confirmedTopics.join("|")}`}>
                  Confirmed topics.
                </Text>
              </>
            ) : null}
            {demandE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`demand-e2e-error:${demandE2E.reason}`}>
                HelpIntent E2E failed safely.
              </Text>
            ) : null}
          </View>
        ) : null}

        {accessibilityE2EEnabled ? (
          <View
            style={styles.accessibilityProbe}
            accessibilityRole="summary"
            accessibilityLabel="a11y-critical-journey"
          >
            <Text
              allowFontScaling
              accessibilityLabel={`a11y-font-scale:${fontScale.toFixed(2)}`}
            >
              Accessibility runtime probe
            </Text>
            <Text
              allowFontScaling
              accessibilityLabel={`a11y-reduced-motion:${reducedMotionEnabled}`}
            >
              Reduced motion preference: {String(reducedMotionEnabled)}
            </Text>
            <Pressable
              accessible
              accessibilityRole="button"
              accessibilityLabel="a11y-action-primary"
              accessibilityHint="Activates the primary critical action"
              style={styles.accessibilityAction}
              onPress={() => undefined}
            >
              <Text allowFontScaling>Основное действие</Text>
            </Pressable>
            <Pressable
              accessible
              accessibilityRole="button"
              accessibilityLabel="a11y-action-secondary"
              accessibilityHint="Activates the secondary critical action"
              style={styles.accessibilityAction}
              onPress={() => undefined}
            >
              <Text allowFontScaling>Дополнительное действие</Text>
            </Pressable>
          </View>
        ) : null}

        {showCompatibilityNotice ? (
          <View style={styles.capability} accessibilityRole="alert">
            <Text accessibilityLabel={`compatibility-e2e:${compatibility.status}`}>
              Совместимость приложения: {compatibility.status}
            </Text>
            {compatibility.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`compatibility-status:${compatibility.decision.status}`}>
                  {compatibility.decision.status === "SUPPORTED"
                    ? "Версия приложения поддерживается."
                    : compatibility.decision.status === "DEPRECATED_BUT_SUPPORTED"
                      ? "Версия приложения пока поддерживается, но доступно обновление."
                      : "Нужно обновить приложение, чтобы безопасно продолжить. Данные не изменены."}
                </Text>
                <Text accessibilityLabel={`compatibility-reason:${compatibility.decision.reason_code}`}>
                  {compatibility.decision.reason_code}
                </Text>
                <Text accessibilityLabel={`compatibility-policy:${compatibility.decision.policy_version}`}>
                  Policy: {compatibility.decision.policy_version}
                </Text>
                <Text accessibilityLabel={`compatibility-contract:${compatibility.decision.contract_version}`}>
                  Contract: {compatibility.decision.contract_version}
                </Text>
                {compatibility.decision.status === "UPDATE_REQUIRED" ? (
                  <>
                    <Text accessibilityLabel={`compatibility-update-reason:${compatibility.decision.update_reason}`}>
                      {compatibility.decision.update_reason}
                    </Text>
                    <Pressable
                      accessibilityRole="button"
                      accessibilityLabel="compatibility-update-action"
                      onPress={() => {
                        if (compatibility.decision.update_url) {
                          void Linking.openURL(compatibility.decision.update_url);
                        }
                      }}
                    >
                      <Text>Обновить приложение</Text>
                    </Pressable>
                  </>
                ) : null}
              </>
            ) : null}
            {compatibility.status === "FAIL" ? (
              <Text accessibilityLabel={`compatibility-error:${compatibility.reason}`}>
                Не удалось безопасно проверить совместимость. Повторите попытку позже. Данные не изменены.
              </Text>
            ) : null}
          </View>
        ) : null}

        {remoteConfigRequired ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`remote-config-e2e:${remoteConfig.status}`}>
              Remote config: {remoteConfig.status}
            </Text>
            {remoteConfig.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`remote-config-source:${remoteConfig.source}`}>
                  Source: {remoteConfig.source}
                </Text>
                <Text accessibilityLabel={`remote-config-reason:${remoteConfig.reasonCode}`}>
                  {remoteConfig.reasonCode}
                </Text>
                <Text
                  accessibilityLabel={`remote-config-capability:REALTIME_CONSULTATION:${
                    remoteConfig.disabledCapabilities.includes("REALTIME_CONSULTATION")
                      ? "DISABLED"
                      : "ENABLED"
                  }`}
                >
                  Realtime capability policy applied.
                </Text>
              </>
            ) : null}
          </View>
        ) : null}

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
                <Text accessibilityLabel={`realtime-network-transport:${realtimeE2E.networkTransport}`}>Transport: {realtimeE2E.networkTransport}</Text>
                <Text accessibilityLabel={`realtime-network-transports-observed:${realtimeE2E.networkTransportsObserved.join("|")}`}>Observed transports recorded.</Text>
                <Text accessibilityLabel={`realtime-screen-state:${realtimeE2E.screenState}`}>Screen: {realtimeE2E.screenState}</Text>
                <Text accessibilityLabel={`realtime-join-auth-state:${realtimeE2E.joinAuthState}`}>Join auth: {realtimeE2E.joinAuthState}</Text>
                <Text accessibilityLabel={`realtime-consultation-id:${realtimeE2E.consultationId}`}>Consultation: {realtimeE2E.consultationId}</Text>
                <Text accessibilityLabel={`realtime-provider-actions:${realtimeE2E.providerActions.join("|")}`}>Provider actions recorded.</Text>
                <Text accessibilityLabel={`realtime-action-connect:${realtimeE2E.providerActions.includes("CONNECT_PROVIDER")}`}>Connect action observed.</Text>
                <Text accessibilityLabel={`realtime-action-reconnect:${realtimeE2E.providerActions.includes("RECONNECT_PROVIDER")}`}>Reconnect action observed.</Text>
                <Text accessibilityLabel={`realtime-action-pause:${realtimeE2E.providerActions.includes("PAUSE_MEDIA")}`}>Pause action observed.</Text>
                <Text accessibilityLabel={`realtime-action-route:${realtimeE2E.providerActions.includes("REFRESH_AUDIO_ROUTE")}`}>Audio-route action observed.</Text>
                <Text accessibilityLabel={`realtime-action-auth:${realtimeE2E.providerActions.includes("REFRESH_JOIN_AUTH")}`}>Join-auth refresh observed.</Text>
              </>
            ) : null}
            {realtimeE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`realtime-e2e-error:${realtimeE2E.reason}`}>Realtime lifecycle failed safely.</Text>
            ) : null}
          </View>
        ) : null}

        {workspaceE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`workspace-e2e:${workspaceE2E.status}`}>
              Workspace E2E: {workspaceE2E.status}
            </Text>
            {workspaceE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`workspace-e2e-identity:${workspaceE2E.identityID}`}>
                  Identity preserved across workspaces.
                </Text>
                <Text accessibilityLabel={`workspace-e2e-kinds:${workspaceE2E.workspaceKinds.join("|")}`}>
                  CLIENT → SPECIALIST → ORGANIZATION
                </Text>
                <Text accessibilityLabel={`workspace-e2e-foreign-denied:${workspaceE2E.foreignWorkspaceDenied}`}>
                  Foreign workspace denied.
                </Text>
              </>
            ) : null}
            {workspaceE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`workspace-e2e-error:${workspaceE2E.reason}`}>
                Workspace switching failed safely.
              </Text>
            ) : null}
          </View>
        ) : null}

        {deletionE2E.status !== "IDLE" ? (
          <View style={styles.capability} accessibilityRole="summary">
            <Text accessibilityLabel={`deletion-e2e:${deletionE2E.status}`}>
              Account deletion E2E: {deletionE2E.status}
            </Text>
            {deletionE2E.status === "PASS" ? (
              <>
                <Text accessibilityLabel={`deletion-e2e-state:${deletionE2E.state}`}>
                  Deletion state: {deletionE2E.state}
                </Text>
                <Text accessibilityLabel={`deletion-e2e-deactivation:${deletionE2E.deactivation}`}>
                  Deactivation: {String(deletionE2E.deactivation)}
                </Text>
                <Text accessibilityLabel={`deletion-e2e-profile-erased:${deletionE2E.profileErased}`}>
                  Profile erased.
                </Text>
                <Text accessibilityLabel={`deletion-e2e-ledger-retained:${deletionE2E.ledgerRetained}`}>
                  Ledger retained.
                </Text>
                <Text accessibilityLabel={`deletion-e2e-idempotent:${deletionE2E.replayIdempotent}`}>
                  Replay idempotent.
                </Text>
              </>
            ) : null}
            {deletionE2E.status === "FAIL" ? (
              <Text accessibilityLabel={`deletion-e2e-error:${deletionE2E.reason}`}>
                Account deletion failed safely.
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
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  card: { gap: 12, padding: 24, paddingBottom: 48 },
  eyebrow: { fontWeight: "700", textTransform: "uppercase", letterSpacing: 1.2 },
  title: { fontWeight: "800", fontSize: 48 },
  body: { fontSize: 18, lineHeight: 27 },
  capability: { gap: 8, marginTop: 12 },
  capabilityTitle: { fontSize: 20, fontWeight: "700" },
  demandCard: { gap: 10, marginTop: 12 },
  demandInput: { minHeight: 96, borderWidth: 1, borderRadius: 8, padding: 12, textAlignVertical: "top" },
  demandAction: { minHeight: 48, justifyContent: "center", paddingHorizontal: 12, borderWidth: 1, borderRadius: 8 },
  demandTopic: { minHeight: 44, justifyContent: "center", paddingHorizontal: 12, borderWidth: 1, borderRadius: 8 },
  accessibilityProbe: { gap: 8, marginTop: 12 },
  accessibilityAction: {
    minHeight: 48,
    minWidth: 48,
    justifyContent: "center",
    paddingHorizontal: 12,
  },
});