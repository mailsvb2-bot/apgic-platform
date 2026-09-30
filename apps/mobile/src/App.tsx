import React from "react";
import { SafeAreaView, StyleSheet, Text, View } from "react-native";

import {
  decideCapability,
  type CapabilityState,
  type DeviceCapability,
} from "./device-capability.ts";

type AppProps = {
  deviceCapability?: DeviceCapability;
  deviceCapabilityState?: CapabilityState;
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
}: AppProps) {
  const decision = decideCapability(deviceCapabilityState);

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
