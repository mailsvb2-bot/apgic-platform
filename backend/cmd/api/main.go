package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
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
		CommitSHA:          os.Getenv("APGIC_COMMIT_SHA"),
		ReleaseTrack:       releaseTrack,
		Demand:             demandService,
		ClientSessionKey:            clientSessionKey,
		ClientCompatibilityPolicies: clientCompatibilityPolicies,
		ReadinessCheck:     readinessCheck,
		LegalAcceptances:   storage,
		Installations:      storage,
		Notifications:      storage,
		ClientMutations:    storage,
		DeepLinks:          storage,
		DeepLinkSigningKey: deepLinkSigningKey,
		Specialists:        storage,
		OrganizationAuth:   storage,
		Organizations:      storage,
		ProductOwnership:   storage,
		Products:           storage,
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
		strings.TrimSpace(os.Getenv("APGIC_IOS_UPDATE_URL")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_MIN_VERSION")),
		strings.TrimSpace(os.Getenv("APGIC_ANDROID_RECOMMENDED_VERSION")),
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
		policy := clientcompat.Policy{
			Platform:                  platform,
			MinimumSupported:          minimum,
			Recommended:               recommended,
			ContractVersion:           contractVersion,
			SupportedContractVersions: supportedContracts,
			PolicyVersion:             policyVersion,
			MinimumUpdateReason:       forcedReason,
			UpdateURL:                 strings.TrimSpace(os.Getenv(prefix + "_UPDATE_URL")),
		}
		if _, err := clientcompat.EvaluateClient(recommended, contractVersion, policy); err != nil {
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
