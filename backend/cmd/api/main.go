package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/remoteconfig"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func main() {
	var readinessCheck func(context.Context) error
	var storage *runtimepostgres.Checker
	var ledgerStore demand.LedgerStore
	var journeyStore demand.JourneyStore
	environment := os.Getenv("APGIC_ENVIRONMENT")
	releaseTrack := envOr("APGIC_RELEASE_TRACK", "R0")
	clientSessionKey := []byte(os.Getenv("APGIC_CLIENT_SESSION_KEY"))
	deepLinkSigningKey := []byte(os.Getenv("APGIC_DEEPLINK_SIGNING_KEY"))
	clientCompatibilityPolicies, err := clientCompatibilityPoliciesFromEnvironment(environment)
	if err != nil {
		log.Fatalf("APGIC mobile compatibility configuration failed: %v", err)
	}
	remoteConfigProvider, err := remoteConfigProviderFromEnvironment(environment)
	if err != nil {
		log.Fatalf("APGIC remote config configuration failed: %v", err)
	}
	providerWebhookKeys, err := providerWebhookKeysFromEnvironment()
	if err != nil {
		log.Fatalf("APGIC provider webhook public-key configuration failed: %v", err)
	}
	consentPolicies, err := consentPoliciesFromEnvironment(environment)
	if err != nil {
		log.Fatalf("APGIC consent policy configuration failed: %v", err)
	}
	if releaseTrack != "R0" {
		if _, err := mobile.NewDeepLinkTokenManager(deepLinkSigningKey); err != nil {
			log.Fatalf("APGIC deep-link signing configuration failed for %s: %v", releaseTrack, err)
		}
	}
	if runtimepostgres.RequiresDatabase(environment) {
		if err := httpapi.ValidateClientSessionKey(clientSessionKey); err != nil {
			log.Fatalf("APGIC client session configuration failed: %v", err)
		}
		var err error
		storage, err = runtimepostgres.Open(context.Background(), os.Getenv("APGIC_DATABASE_URL"))
		if err != nil {
			log.Fatalf("APGIC PostgreSQL readiness failed: %v", err)
		}
		defer storage.Close()
		readinessCheck = storage.Ready
		ledgerStore = storage
		journeyStore = storage
	}
	var demandService *demand.Service
	if journeyStore != nil {
		var err error
		demandService, err = demand.NewConformanceServiceWithStoresAndSpecialists(nil, ledgerStore, journeyStore, storage)
		if err != nil {
			log.Fatalf("APGIC journey hydration failed: %v", err)
		}
	} else {
		demandService = demand.NewConformanceServiceWithLedgerStore(nil, ledgerStore)
	}
	handler := httpapi.New(httpapi.Options{
		CommitSHA:                   os.Getenv("APGIC_COMMIT_SHA"),
		ConformanceProviderEvents:   !runtimepostgres.RequiresDatabase(environment) && os.Getenv("APGIC_CONFORMANCE_PROVIDER_EVENTS") == "1",
		ProviderWebhookKeys:         providerWebhookKeys,
		ReleaseTrack:                releaseTrack,
		Demand:                      demandService,
		ClientSessionKey:            clientSessionKey,
		ClientCompatibilityPolicies: clientCompatibilityPolicies,
		RemoteConfigProvider:        remoteConfigProvider,
		ReadinessCheck:              readinessCheck,
		LegalAcceptances:            storage,
		Consents:                    storage,
		ConsentPolicies:             consentPolicies,
		Installations:               storage,
		MobileWorkspaces:            storage,
		Notifications:               storage,
		ClientMutations:             storage,
		DeepLinks:                   storage,
		DeepLinkSigningKey:          deepLinkSigningKey,
		Specialists:                 storage,
		OrganizationAuth:            storage,
		Organizations:               storage,
		ProductOwnership:            storage,
		Products:                    storage,
		LaunchConfig: launchconfig.Config{
			JurisdictionMatrixVersion: os.Getenv("APGIC_JURISDICTION_MATRIX_VERSION"),
			RetentionPolicyVersion:    os.Getenv("APGIC_RETENTION_POLICY_VERSION"),
			SLOPolicyVersion:          os.Getenv("APGIC_SLO_POLICY_VERSION"),
			ProviderMatrixVersion:     os.Getenv("APGIC_PROVIDER_MATRIX_VERSION"),
		},
	})

	addr := envOr("APGIC_HTTP_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("APGIC API listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

type providerWebhookKeyResolver map[string]ed25519.PublicKey

func (r providerWebhookKeyResolver) ResolveWebhookPublicKey(connectorInstanceID, keyID string) (ed25519.PublicKey, bool) {
	key, ok := r[strings.TrimSpace(connectorInstanceID)+"/"+strings.TrimSpace(keyID)]
	return key, ok
}


func consentPoliciesFromEnvironment(environment string) (map[string]privacy.ConsentPolicy, error) {
	version := strings.TrimSpace(os.Getenv("APGIC_GROWTH_CONSENT_POLICY_VERSION"))
	textVersion := strings.TrimSpace(os.Getenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION"))
	if version == "" && textVersion == "" {
		if runtimepostgres.RequiresDatabase(environment) {
			return nil, fmt.Errorf("growth consent policy is required in %s", environment)
		}
		return nil, nil
	}
	if version == "" || textVersion == "" || version == "CONFIG_REQUIRED" || textVersion == "CONFIG_REQUIRED" {
		return nil, fmt.Errorf("growth consent policy configuration is incomplete")
	}
	policy, err := privacy.NewConsentPolicy(privacy.PurposeGrowthSessionProjection, version, textVersion)
	if err != nil {
		return nil, err
	}
	return map[string]privacy.ConsentPolicy{policy.Purpose: policy}, nil
}

func providerWebhookKeysFromEnvironment() (connector.WebhookPublicKeyResolver, error) {
	raw := strings.TrimSpace(os.Getenv("APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON"))
	if raw == "" {
		return nil, nil
	}
	var configured map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		return nil, fmt.Errorf("decode APGIC_PROVIDER_WEBHOOK_PUBLIC_KEYS_JSON: %w", err)
	}
	resolver := providerWebhookKeyResolver{}
	for connectorID, keys := range configured {
		connectorID = strings.TrimSpace(connectorID)
		if connectorID == "" || len(keys) == 0 {
			return nil, fmt.Errorf("provider webhook connector id/key set must not be empty")
		}
		for keyID, encoded := range keys {
			keyID = strings.TrimSpace(keyID)
			if keyID == "" {
				return nil, fmt.Errorf("provider webhook key id must not be empty")
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
			if err != nil || len(decoded) != ed25519.PublicKeySize {
				return nil, fmt.Errorf("provider webhook public key %s/%s must be base64 Ed25519 public key", connectorID, keyID)
			}
			resolver[connectorID+"/"+keyID] = append(ed25519.PublicKey(nil), decoded...)
		}
	}
	if len(resolver) == 0 {
		return nil, fmt.Errorf("provider webhook public-key set must not be empty")
	}
	return resolver, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func clientCompatibilityPoliciesFromEnvironment(environment string) (map[clientcompat.Platform]clientcompat.Policy, error) {
	policyVersion := strings.TrimSpace(os.Getenv("APGIC_MOBILE_POLICY_VERSION"))
	contractVersion := strings.TrimSpace(os.Getenv("APGIC_MOBILE_CONTRACT_VERSION"))
	supportedContractsRaw := strings.TrimSpace(os.Getenv("APGIC_MOBILE_SUPPORTED_CONTRACTS"))
	forcedReasonRaw := strings.TrimSpace(os.Getenv("APGIC_MOBILE_FORCED_UPDATE_REASON"))

	values := []string{
		policyVersion,
		contractVersion,
		supportedContractsRaw,
		forcedReasonRaw,
		strings.TrimSpace(os.Getenv("APGIC_IOS_MIN_VERSION")),
		strings.TrimSpace(os.Getenv("APGIC_IOS_RECOMMENDED_VERSION")),
		strings.TrimSpace(os.Getenv("APGIC_IOS_MIN_BUILD")),
		strings.TrimSpace(os.Getenv("APGIC_IOS_UPDATE_URL")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_MIN_VERSION")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_RECOMMENDED_VERSION")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_MIN_BUILD")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_UPDATE_URL")),
	}
	anyConfigured := false
	for _, value := range values {
		if value != "" {
			anyConfigured = true
			break
		}
	}
	if !anyConfigured {
		if runtimepostgres.RequiresDatabase(environment) {
			return nil, fmt.Errorf("versioned mobile compatibility policy is required in %s", environment)
		}
		return nil, nil
	}
	for _, value := range values {
		if value == "" {
			return nil, fmt.Errorf("mobile compatibility policy is incomplete")
		}
	}

	supportedContracts := splitCSV(supportedContractsRaw)
	if len(supportedContracts) == 0 {
		return nil, fmt.Errorf("supported contract versions are required")
	}
	forcedReason := clientcompat.ForcedUpdateReason(forcedReasonRaw)
	switch forcedReason {
	case clientcompat.SecurityCritical, clientcompat.LegalCritical, clientcompat.IncompatibleCritical:
	default:
		return nil, fmt.Errorf("unsupported forced-update reason %q", forcedReasonRaw)
	}

	buildPolicy := func(platform clientcompat.Platform, prefix string) (clientcompat.Policy, error) {
		minimum, err := clientcompat.ParseVersion(os.Getenv(prefix + "_MIN_VERSION"))
		if err != nil {
			return clientcompat.Policy{}, fmt.Errorf("%s minimum version: %w", platform, err)
		}
		recommended, err := clientcompat.ParseVersion(os.Getenv(prefix + "_RECOMMENDED_VERSION"))
		if err != nil {
			return clientcompat.Policy{}, fmt.Errorf("%s recommended version: %w", platform, err)
		}
		minimumBuild, err := strconv.Atoi(strings.TrimSpace(os.Getenv(prefix + "_MIN_BUILD")))
		if err != nil || minimumBuild <= 0 {
			return clientcompat.Policy{}, fmt.Errorf("%s minimum build must be a positive integer", platform)
		}
		policy := clientcompat.Policy{
			Platform:                  platform,
			MinimumSupported:          minimum,
			Recommended:               recommended,
			MinimumBuild:              minimumBuild,
			ContractVersion:           contractVersion,
			SupportedContractVersions: supportedContracts,
			PolicyVersion:             policyVersion,
			MinimumUpdateReason:       forcedReason,
			UpdateURL:                 strings.TrimSpace(os.Getenv(prefix + "_UPDATE_URL")),
		}
		if _, err := clientcompat.EvaluateClient(recommended, minimumBuild, contractVersion, policy); err != nil {
			return clientcompat.Policy{}, fmt.Errorf("%s policy: %w", platform, err)
		}
		return policy, nil
	}

	ios, err := buildPolicy(clientcompat.IOS, "APGIC_IOS")
	if err != nil {
		return nil, err
	}
	android, err := buildPolicy(clientcompat.Android, "APGIC_ANDROID")
	if err != nil {
		return nil, err
	}
	return map[clientcompat.Platform]clientcompat.Policy{
		clientcompat.IOS:     ios,
		clientcompat.Android: android,
	}, nil
}

func splitCSV(raw string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func remoteConfigProviderFromEnvironment(environment string) (func(time.Time) (remoteconfig.SignedEnvelope, error), error) {
	keyID := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_KEY_ID"))
	privateKeyRaw := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_PRIVATE_KEY_BASE64"))
	versionRaw := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_VERSION"))
	policyID := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_POLICY_ID"))
	ttlRaw := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_TTL_SECONDS"))
	disabledRaw := strings.TrimSpace(os.Getenv("APGIC_REMOTE_CONFIG_DISABLED_CAPABILITIES"))

	values := []string{keyID, privateKeyRaw, versionRaw, policyID, ttlRaw}
	anyConfigured := false
	for _, value := range values {
		if value != "" {
			anyConfigured = true
			break
		}
	}
	if !anyConfigured {
		if runtimepostgres.RequiresDatabase(environment) {
			return nil, fmt.Errorf("signed remote config is required in %s", environment)
		}
		return nil, nil
	}
	for _, value := range values {
		if value == "" {
			return nil, fmt.Errorf("remote config is incomplete")
		}
	}

	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("remote config private key must be valid base64")
	}
	var privateKey ed25519.PrivateKey
	switch len(privateKeyBytes) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(privateKeyBytes)
	case ed25519.PrivateKeySize:
		privateKey = ed25519.PrivateKey(privateKeyBytes)
	default:
		return nil, fmt.Errorf("remote config private key must decode to 32-byte seed or 64-byte Ed25519 private key")
	}
	version, err := strconv.ParseUint(versionRaw, 10, 64)
	if err != nil || version == 0 {
		return nil, fmt.Errorf("remote config version must be a positive integer")
	}
	ttlSeconds, err := strconv.Atoi(ttlRaw)
	if err != nil || ttlSeconds < 60 || ttlSeconds > 604800 {
		return nil, fmt.Errorf("remote config TTL must be between 60 and 604800 seconds")
	}

	disabled := make([]remoteconfig.Capability, 0)
	reasons := make(map[remoteconfig.Capability]string)
	for _, item := range splitCSV(disabledRaw) {
		parts := strings.SplitN(item, ":", 2)
		capability := remoteconfig.Capability(strings.TrimSpace(parts[0]))
		disabled = append(disabled, capability)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			reasons[capability] = strings.TrimSpace(parts[1])
		}
	}
	publisher, err := remoteconfig.NewPublisher(
		keyID,
		privateKey,
		version,
		policyID,
		time.Duration(ttlSeconds)*time.Second,
		disabled,
		reasons,
	)
	if err != nil {
		return nil, err
	}
	return publisher.Envelope, nil
}
