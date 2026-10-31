package runtimepostgres

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeletionProviderConfig is deploy-time configuration, never sent to clients.
type DeletionProviderConfig struct {
	URL             string `json:"url"`
	BearerToken     string `json:"bearer_token"`
	PublicKeyBase64 string `json:"public_key_base64"`
}

type deletionProvider struct {
	endpoint string
	token    string
	key      ed25519.PublicKey
	client   *http.Client
}

// ConfiguredDeletionProviders accepts only explicit HTTPS endpoints with an
// independently provisioned signature key and per-provider bearer credential.
func ConfiguredDeletionProviders(raw []byte) (map[string]deletionProvider, error) {
	var cfg map[string]DeletionProviderConfig
	if len(raw) == 0 {
		return nil, errors.New("deletion provider configuration is missing")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	result := make(map[string]deletionProvider, len(cfg))
	for ref, item := range cfg {
		endpoint, err := url.Parse(item.URL)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" ||
			endpoint.User != nil || endpoint.Fragment != "" ||
			len(item.BearerToken) < 24 || strings.TrimSpace(ref) != ref ||
			ref == "" {
			return nil, fmt.Errorf("invalid deletion provider configuration: %q", ref)
		}
		pub, err := base64.StdEncoding.Strict().DecodeString(item.PublicKeyBase64)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid deletion provider public key: %q", ref)
		}
		result[ref] = deletionProvider{
			endpoint: endpoint.String(),
			token:    item.BearerToken,
			key:      ed25519.PublicKey(pub),
			client: &http.Client{
				Timeout: 15 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		}
	}
	return result, nil
}

func (p deletionProvider) erase(ctx context.Context, delivery ProviderErasureDelivery) ([]byte, error) {
	body, err := json.Marshal(struct {
		RequestID   string `json:"request_id"`
		IdentityID  string `json:"identity_id"`
		ProviderRef string `json:"provider_ref"`
	}{
		RequestID: delivery.RequestID, IdentityID: delivery.IdentityID,
		ProviderRef: delivery.ProviderRef,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Idempotency-Key", "apgic-deletion:"+delivery.RequestID+":"+delivery.ProviderRef)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.New("deletion provider returned a non-success status")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8193))
	if err != nil || len(raw) > 8192 {
		return nil, errors.New("provider attestation exceeds allowed size")
	}
	return raw, nil
}
