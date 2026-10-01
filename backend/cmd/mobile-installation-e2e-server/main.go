package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

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

func main() {
	addr := os.Getenv("APGIC_MOBILE_E2E_ADDR")
	if addr == "" {
		addr = "127.0.0.1:43113"
	}
	key := []byte(strings.Repeat("e", 32))
	handler := httpapi.New(httpapi.Options{
		Demand:           demand.NewConformanceService(nil),
		Installations:    newConformanceInstallationStore(),
		ClientSessionKey: key,
	})
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
