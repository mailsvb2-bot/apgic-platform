import React from "react";
import { SafeAreaView, StyleSheet, Text, View } from "react-native";

export default function App() {
  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.card} accessibilityRole="summary">
        <Text style={styles.eyebrow}>R0 · Foundation</Text>
        <Text style={styles.title}>APGIC</Text>
        <Text style={styles.body}>
          Одна Identity и одна server truth для iOS, Android, Web и PWA.
        </Text>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, justifyContent: "center", padding: 24 },
  card: { gap: 12 },
  eyebrow: { fontWeight: "700", textTransform: "uppercase", letterSpacing: 1.2 },
  title: { fontWeight: "800", fontSize: 48 },
  body: { fontSize: 18, lineHeight: 27 }
});
