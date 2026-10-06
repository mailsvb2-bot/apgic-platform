package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientcompat"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/remoteconfig"
)

const (
	authzOwnOrganizationID     = "00000000-0000-0000-0000-00000000a001"
	authzForeignOrganizationID = "00000000-0000-0000-0000-00000000b001"
	authzOwnPrivateName        = "AUTH001 OWN PRIVATE"
	authzForeignPrivateName    = "TOP SECRET AUTH001 FOREIGN"
)

type conformanceOrganizationAuthStore struct {
	mu      sync.Mutex
	records map[string]audit.Record
}

func newConformanceOrganizationAuthStore() *conformanceOrganizationAuthStore {
	return &conformanceOrganizationAuthStore{records: make(map[string]audit.Record)}
}

func (s *conformanceOrganizationAuthStore) ActiveOrganizationMembership(identityID, organizationID string) (bool, error) {
	return strings.TrimSpace(identityID) != "" && strings.TrimSpace(organizationID) == authzOwnOrganizationID, nil
}

func (s *conformanceOrganizationAuthStore) OrganizationPrivateName(organizationID string) (string, bool, error) {
	switch strings.TrimSpace(organizationID) {
	case authzOwnOrganizationID:
		return authzOwnPrivateName, true, nil
	case authzForeignOrganizationID:
		return authzForeignPrivateName, true, nil
	default:
		return "", false, nil
	}
}

func (s *conformanceOrganizationAuthStore) Append(record audit.Record) error {
	validated, err := audit.New(record)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[validated.CorrelationID] = validated
	return nil
}

func (s *conformanceOrganizationAuthStore) auditByCorrelation(correlationID string) (audit.Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[correlationID]
	return record, ok
}

type conformanceE2EHandler struct {
	next  http.Handler
	authz *conformanceOrganizationAuthStore
}

func (h *conformanceE2EHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/e2e/authz-audit/"
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, prefix) {
		correlationID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
		record, ok := h.authz.auditByCorrelation(correlationID)
		if !ok || correlationID == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"correlation_id": record.CorrelationID,
			"reason":         record.Reason,
			"decision":       "DENY",
			"scope":          record.Scope,
			"resource_ref":   record.ResourceRef,
		})
		return
	}
	h.next.ServeHTTP(w, r)
}

type conformanceWorkspaceStore struct{}

func (conformanceWorkspaceStore) MobileWorkspaces(identityID string) ([]mobile.Workspace, error) {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return nil, mobile.ErrWorkspaceIdentityNotFound
	}
	return []mobile.Workspace{
		{
			ID:         mobile.ClientWorkspaceID(identityID),
			IdentityID: identityID,
			TenantID:   identityID,
			Kind:       mobile.WorkspaceClient,
		},
		{
			ID:         mobile.SpecialistWorkspaceID("e2e-specialist"),
			IdentityID: identityID,
			TenantID:   identityID,
			Kind:       mobile.WorkspaceSpecialist,
		},
		{
			ID:         mobile.OrganizationWorkspaceID("e2e-organization"),
			IdentityID: identityID,
			TenantID:   "e2e-organization",
			Kind:       mobile.WorkspaceOrganization,
		},
	}, nil
}

func (s conformanceWorkspaceStore) MobileWorkspace(identityID, workspaceID string) (mobile.Workspace, bool, error) {
	workspaces, err := s.MobileWorkspaces(identityID)
	if err != nil {
		return mobile.Workspace{}, false, err
	}
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			return workspace, true, nil
		}
	}
	return mobile.Workspace{}, false, nil
}

type conformanceInstallationStore struct {
	mu     sync.Mutex
	values map[string]mobile.ClientInstallation
}

func newConformanceInstallationStore() *conformanceInstallationStore {
	return &conformanceInstallationStore{values: make(map[string]mobile.ClientInstallation)}
}

func (s *conformanceInstallationStore) RegisterInstallation(input mobile.ClientInstallation) (mobile.ClientInstallation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	canonical, err := mobile.NewInstallation(input.ID, input.IdentityID, input.Platform, input.PushEndpoint, input.UpdatedAt)
	if err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if existing, ok := s.values[canonical.ID]; ok {
		if existing.IdentityID != canonical.IdentityID || existing.Platform != canonical.Platform || existing.State != mobile.InstallationActive || existing.PushEndpoint != canonical.PushEndpoint {
			return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
		}
		return existing, true, nil
	}
	for id, existing := range s.values {
		if existing.State != mobile.InstallationActive || existing.PushEndpoint != canonical.PushEndpoint {
			continue
		}
		if existing.IdentityID != canonical.IdentityID {
			return mobile.ClientInstallation{}, false, mobile.ErrPushEndpointAlreadyInUse
		}
		if id != canonical.ID {
			if err := existing.Revoke(canonical.UpdatedAt); err != nil {
				return mobile.ClientInstallation{}, false, err
			}
			s.values[id] = existing
		}
	}
	s.values[canonical.ID] = canonical
	return canonical, false, nil
}

func (s *conformanceInstallationStore) RotateInstallationPushEndpoint(identityID, installationID, endpoint string, now time.Time) (mobile.ClientInstallation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.values[installationID]
	if !ok || current.IdentityID != identityID {
		return mobile.ClientInstallation{}, false, mobile.ErrInstallationNotFound
	}
	if current.State != mobile.InstallationActive || strings.TrimSpace(endpoint) == "" || now.IsZero() {
		return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
	}
	if current.PushEndpoint == strings.TrimSpace(endpoint) {
		return current, true, nil
	}
	for id, existing := range s.values {
		if id == installationID || existing.State != mobile.InstallationActive || existing.PushEndpoint != strings.TrimSpace(endpoint) {
			continue
		}
		if existing.IdentityID != identityID {
			return mobile.ClientInstallation{}, false, mobile.ErrPushEndpointAlreadyInUse
		}
		if err := existing.Revoke(now); err != nil {
			return mobile.ClientInstallation{}, false, err
		}
		s.values[id] = existing
	}
	if err := current.RotatePushEndpoint(endpoint, now); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	s.values[installationID] = current
	return current, false, nil
}

func (s *conformanceInstallationStore) RevokeInstallation(identityID, installationID string, now time.Time) (mobile.ClientInstallation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.values[installationID]
	if !ok || current.IdentityID != identityID {
		return mobile.ClientInstallation{}, false, mobile.ErrInstallationNotFound
	}
	if current.State == mobile.InstallationRevoked {
		return current, true, nil
	}
	if err := current.Revoke(now); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	s.values[installationID] = current
	return current, false, nil
}

func (s *conformanceInstallationStore) ListInstallations(identityID string) ([]mobile.ClientInstallation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(identityID) == "" {
		return nil, mobile.ErrInvalidInstallation
	}
	values := make([]mobile.ClientInstallation, 0)
	for _, value := range s.values {
		if value.IdentityID == identityID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

type conformanceDeepLinkStore struct{}

func (conformanceDeepLinkStore) DeepLinkResource(kind mobile.LinkKind, targetID string) (mobile.DeepLinkResource, bool, error) {
	if kind != mobile.LinkSpecialist || targetID != "e2e-specialist" {
		return mobile.DeepLinkResource{}, false, nil
	}
	return mobile.DeepLinkResource{
		Kind:        mobile.LinkSpecialist,
		TargetID:    targetID,
		AccessClass: mobile.LinkPublicResource,
	}, true, nil
}

type conformanceNotificationStore struct{}

func (conformanceNotificationStore) MobileNotificationDelivery(_ context.Context, identityID, deliveryID string) (notification.MobileDeliveryProjection, bool, error) {
	if strings.TrimSpace(identityID) == "" || deliveryID != "00000000-0000-0000-0000-00000000e701" {
		return notification.MobileDeliveryProjection{}, false, nil
	}
	return notification.MobileDeliveryProjection{
		ContractVersion:  "notification-projection-v1",
		DeliveryID:       deliveryID,
		IntentID:         "00000000-0000-0000-0000-00000000e702",
		Purpose:          "BOOKING_CONFIRMATION",
		RelatedObjectRef: "booking/e2e-booking",
		Channel:          notification.ChannelPush,
		DeliveryState:    notification.DeliveryPending,
		DataClass:        "SENSITIVE",
		PreviewMode:      notification.PreviewGeneric,
	}, true, nil
}

type conformanceMutationRecord struct {
	envelope      mutation.Envelope
	mutationID    string
	state         string
	sideEffectRef string
	failureCode   string
}

type conformanceMutationStore struct {
	mu      sync.Mutex
	records map[string]conformanceMutationRecord
}

func newConformanceMutationStore() *conformanceMutationStore {
	return &conformanceMutationStore{records: make(map[string]conformanceMutationRecord)}
}

func mutationRecordKey(envelope mutation.Envelope) string {
	return envelope.IdentityID + "|" + envelope.Operation + "|" + envelope.IdempotencyKey
}

func (s *conformanceMutationStore) Claim(_ context.Context, mutationID string, envelope mutation.Envelope, _ time.Time) (mutation.ClaimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := mutationRecordKey(envelope)
	if existing, ok := s.records[key]; ok {
		if existing.envelope.CorrelationID != envelope.CorrelationID ||
			existing.envelope.RequestDigest != envelope.RequestDigest {
			return mutation.ClaimResult{
				Outcome:    mutation.OutcomeConflict,
				MutationID: existing.mutationID,
				State:      existing.state,
			}, nil
		}
		outcome := mutation.OutcomeDuplicate
		switch existing.state {
		case mutation.StateApplied:
			outcome = mutation.OutcomeDuplicateApplied
		case mutation.StateFailed:
			outcome = mutation.OutcomeFailed
		}
		return mutation.ClaimResult{
			Outcome:       outcome,
			MutationID:    existing.mutationID,
			State:         existing.state,
			SideEffectRef: existing.sideEffectRef,
			FailureCode:   existing.failureCode,
		}, nil
	}

	s.records[key] = conformanceMutationRecord{
		envelope:   envelope,
		mutationID: mutationID,
		state:      mutation.StateClaimed,
	}
	return mutation.ClaimResult{
		Outcome:    mutation.OutcomeClaimed,
		MutationID: mutationID,
		State:      mutation.StateClaimed,
	}, nil
}

func (s *conformanceMutationStore) Finalize(_ context.Context, mutationID, sideEffectRef string, _ time.Time) (mutation.FinalizeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, record := range s.records {
		if record.mutationID != mutationID {
			continue
		}
		switch record.state {
		case mutation.StateApplied:
			if record.sideEffectRef == sideEffectRef {
				return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_APPLIED"}, nil
			}
			return mutation.FinalizeResult{ReasonCode: "MUTATION_SIDE_EFFECT_CONFLICT"}, nil
		case mutation.StateFailed:
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_FAILED"}, nil
		}
		record.state = mutation.StateApplied
		record.sideEffectRef = sideEffectRef
		s.records[key] = record
		return mutation.FinalizeResult{Changed: true, ReasonCode: "MUTATION_APPLIED"}, nil
	}
	return mutation.FinalizeResult{ReasonCode: "MUTATION_FINALIZE_INVALID"}, nil
}

func (s *conformanceMutationStore) Fail(_ context.Context, mutationID, failureCode string, _ time.Time) (mutation.FinalizeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, record := range s.records {
		if record.mutationID != mutationID {
			continue
		}
		switch record.state {
		case mutation.StateApplied:
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_APPLIED"}, nil
		case mutation.StateFailed:
			return mutation.FinalizeResult{ReasonCode: "MUTATION_ALREADY_FAILED"}, nil
		}
		record.state = mutation.StateFailed
		record.failureCode = failureCode
		s.records[key] = record
		return mutation.FinalizeResult{Changed: true, ReasonCode: "MUTATION_FAILED"}, nil
	}
	return mutation.FinalizeResult{ReasonCode: "MUTATION_FAIL_INVALID"}, nil
}

type loseFirstCheckoutResponse struct {
	next http.Handler
	mu   sync.Mutex
	lost bool
}

func (h *loseFirstCheckoutResponse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/mobile/checkout-instructions" {
		h.next.ServeHTTP(w, r)
		return
	}

	h.mu.Lock()
	lose := !h.lost
	if lose {
		h.lost = true
	}
	h.mu.Unlock()

	if !lose {
		h.next.ServeHTTP(w, r)
		return
	}

	recorder := httptest.NewRecorder()
	h.next.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusCreated {
		for key, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"code":"E2E_RESPONSE_LOST","message_safe":"Committed response intentionally lost for offline retry proof.","correlation_id":"e2e-offline-retry","retryable":true}`))
}

func main() {
	addr := os.Getenv("APGIC_MOBILE_E2E_ADDR")
	if addr == "" {
		addr = "127.0.0.1:43113"
	}
	key := []byte(strings.Repeat("e", 32))
	mutations := newConformanceMutationStore()
	minimum, err := clientcompat.ParseVersion("1.0.0")
	if err != nil {
		log.Fatal(err)
	}
	recommended, err := clientcompat.ParseVersion("1.0.0")
	if err != nil {
		log.Fatal(err)
	}
	compatibilityPolicies := map[clientcompat.Platform]clientcompat.Policy{}
	for _, platform := range []clientcompat.Platform{clientcompat.IOS, clientcompat.Android} {
		compatibilityPolicies[platform] = clientcompat.Policy{
			Platform:                  platform,
			MinimumSupported:          minimum,
			Recommended:               recommended,
			MinimumBuild:              1,
			ContractVersion:           "0.10.0-r0-remote-config",
			SupportedContractVersions: []string{"0.8.0-r2-offline-sync", "0.9.0-r0-mobile-compatibility", "0.10.0-r0-remote-config"},
			PolicyVersion:             "mobile013-e2e-v1",
			MinimumUpdateReason:       clientcompat.IncompatibleCritical,
			UpdateURL:                 "https://apgic.ru/update",
		}
	}
	remoteConfigPublisher, err := remoteconfig.NewPublisher(
		"mobile027-e2e-key",
		ed25519.NewKeyFromSeed([]byte(strings.Repeat("r", ed25519.SeedSize))),
		1,
		"mobile027-e2e-policy-v1",
		30*time.Minute,
		[]remoteconfig.Capability{remoteconfig.CapabilityRealtimeConsultation},
		map[remoteconfig.Capability]string{
			remoteconfig.CapabilityRealtimeConsultation: "INCIDENT_DISABLE_REALTIME",
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	authzStore := newConformanceOrganizationAuthStore()
	canonicalHandler := httpapi.New(httpapi.Options{
		Demand:                      demand.NewConformanceService(nil),
		Installations:               newConformanceInstallationStore(),
		MobileWorkspaces:            conformanceWorkspaceStore{},
		OrganizationAuth:            authzStore,
		Notifications:               conformanceNotificationStore{},
		ClientMutations:             mutations,
		DeepLinks:                   conformanceDeepLinkStore{},
		DeepLinkSigningKey:          key,
		ClientSessionKey:            key,
		ClientCompatibilityPolicies: compatibilityPolicies,
		RemoteConfigProvider:        remoteConfigPublisher.Envelope,
	})
	handler := &loseFirstCheckoutResponse{next: &conformanceE2EHandler{next: canonicalHandler, authz: authzStore}}
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("APGIC mobile installation E2E server listening on %s", addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
