package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"testing"
)

func TestProviderWebhookKeysFromEnvironment(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(
		"APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON",
		`{"payment-connector":{"provider-key-v1":"`+base64.StdEncoding.EncodeToString(publicKey)+`"}}`,
	)

	resolver, err := providerWebhookKeysFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := resolver.ResolveWebhookPublicKey("payment-connector", "provider-key-v1")
	if !ok || string(resolved) != string(publicKey) {
		t.Fatalf("resolved key mismatch: ok=%v len=%d", ok, len(resolved))
	}
	if _, ok := resolver.ResolveWebhookPublicKey("payment-connector", "unknown"); ok {
		t.Fatal("unknown provider webhook key unexpectedly resolved")
	}
}

func TestProviderWebhookKeysFromEnvironmentFailsClosed(t *testing.T) {
	for name, raw := range map[string]string{
		"invalid_json":    "{",
		"empty_set":       "{}",
		"empty_connector": `{"":{"k":"ZmFrZQ=="}}`,
		"empty_key_id":    `{"connector":{"":"ZmFrZQ=="}}`,
		"invalid_key":     `{"connector":{"key":"ZmFrZQ=="}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON", raw)
			if resolver, err := providerWebhookKeysFromEnvironment(); err == nil || resolver != nil {
				t.Fatalf("expected fail-closed config, resolver=%#v err=%v", resolver, err)
			}
		})
	}
}

func TestProviderWebhookKeysFromEnvironmentAllowsNoProviderConfiguration(t *testing.T) {
	if err := os.Unsetenv("APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON"); err != nil {
		t.Fatal(err)
	}
	resolver, err := providerWebhookKeysFromEnvironment()
	if err != nil || resolver != nil {
		t.Fatalf("empty payment-provider config must remain unavailable, resolver=%#v err=%v", resolver, err)
	}
}
