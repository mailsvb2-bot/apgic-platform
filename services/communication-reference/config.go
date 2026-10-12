package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errConfiguration = errors.New("reference CommunicationProvider configuration invalid")

type config struct {
	Listen            string
	APGICIssuerKey    string
	APIKey            string
	APISecret         string
	MediaWebSocketURL string
	PublicOrigin      string
	ProviderID        string
	APGICWebhookURL   string
	KeyID             string
	SigningKey        ed25519.PrivateKey
	StateDir          string
	ClientBundle      string
	MinimumSession    time.Duration
}

func readConfig() (config, error) {
	private, err := base64.StdEncoding.DecodeString(os.Getenv("APGIC_COMM_REF_ED25519_PRIVATE_KEY_B64"))
	if err != nil {
		return config{}, errConfiguration
	}
	c := config{
		Listen:            env("APGIC_COMM_REF_LISTEN", "127.0.0.1:8650"),
		APGICIssuerKey:    os.Getenv("APGIC_COMM_ISSUER_TOKEN"),
		APIKey:            os.Getenv("LIVEKIT_API_KEY"),
		APISecret:         os.Getenv("LIVEKIT_API_SECRET"),
		MediaWebSocketURL: os.Getenv("LIVEKIT_URL"),
		PublicOrigin:      os.Getenv("APGIC_COMM_JOIN_ORIGIN"),
		ProviderID:        os.Getenv("APGIC_COMM_JOIN_PROVIDER_INSTANCE_ID"),
		APGICWebhookURL:   os.Getenv("APGIC_COMM_REF_APGIC_WEBHOOK_URL"),
		KeyID:             os.Getenv("APGIC_COMM_REF_KEY_ID"),
		SigningKey:        ed25519.PrivateKey(private),
		StateDir:          env("APGIC_COMM_REF_STATE_DIR", "./state"),
		ClientBundle:      env("APGIC_COMM_REF_LIVEKIT_CLIENT_BUNDLE", "./assets/livekit-client.umd.min.js"),
		MinimumSession:    60 * time.Second,
	}
	if len(c.SigningKey) != ed25519.PrivateKeySize ||
		len(c.APGICIssuerKey) < 24 || len(c.APISecret) < 32 ||
		c.APIKey == "" || c.ProviderID == "" || c.KeyID == "" {
		return config{}, errConfiguration
	}
	if !validUUID(c.ProviderID) {
		return config{}, errConfiguration
	}
	public, e1 := url.Parse(c.PublicOrigin)
	media, e2 := url.Parse(c.MediaWebSocketURL)
	webhook, e3 := url.Parse(c.APGICWebhookURL)
	if e1 != nil || e2 != nil || e3 != nil ||
		public.Scheme != "https" || public.Hostname() == "" || public.User != nil ||
		(public.Path != "" && public.Path != "/") || public.RawQuery != "" || public.Fragment != "" ||
		media.Scheme != "wss" || media.Hostname() == "" || media.User != nil ||
		webhook.Scheme != "https" || webhook.Hostname() == "" || webhook.User != nil ||
		webhook.RawQuery != "" || webhook.Fragment != "" {
		return config{}, errConfiguration
	}
	if !strings.HasSuffix(webhook.Path, "/v1/consultation-provider-webhooks") {
		return config{}, errConfiguration
	}
	if strings.TrimSpace(c.StateDir) == "" || strings.TrimSpace(c.ClientBundle) == "" {
		return config{}, errConfiguration
	}
	c.StateDir = filepath.Clean(c.StateDir)
	return c, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
