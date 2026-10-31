package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

// DeletionRuntime is the only authorized execution route for account erasure.
// No HTTP client can call it or manufacture its legal/provider evidence.
type DeletionRuntime struct {
	store           *Checker
	signedPolicy    []byte
	approvalKey     ed25519.PublicKey
	expectedVersion string
	providers       map[string]deletionProvider
}

func NewDeletionRuntime(store *Checker, signedPolicy []byte, approvalKey ed25519.PublicKey, expectedVersion string, rawProviders []byte) (*DeletionRuntime, error) {
	if store == nil || len(signedPolicy) == 0 {
		return nil, errors.New("deletion execution requires canonical PostgreSQL and signed policy")
	}
	p, err := privacy.VerifyDeletionPolicy(signedPolicy, approvalKey, expectedVersion, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var providers map[string]deletionProvider
	if p.NoExternalData {
		providers = make(map[string]deletionProvider)
	} else {
		providers, err = ConfiguredDeletionProviders(rawProviders)
		if err != nil {
			return nil, err
		}
		for _, ref := range p.ProviderRefs {
			if _, ok := providers[ref]; !ok {
				return nil, fmt.Errorf("signed deletion policy references an unconfigured provider: %s", ref)
			}
		}
	}
	return &DeletionRuntime{store: store, signedPolicy: append([]byte(nil), signedPolicy...),
		approvalKey: append(ed25519.PublicKey(nil), approvalKey...), expectedVersion: expectedVersion,
		providers: providers}, nil
}

// Cycle progresses work in bounded batches. A provider exception is recorded
// for durable retry rather than making the whole service drop an unfinished job.
func (d *DeletionRuntime) Cycle(ctx context.Context) error {
	if d == nil || d.store == nil {
		return errors.New("deletion executor unavailable")
	}
	// Verify expiry on EVERY cycle, not just once at startup.
	if _, err := privacy.VerifyDeletionPolicy(d.signedPolicy, d.approvalKey, d.expectedVersion, time.Now().UTC()); err != nil {
		return err
	}
	for i := 0; i < 16; i++ {
		moved, err := d.store.AdvanceApprovedDeletion(ctx, d.signedPolicy, d.approvalKey, d.expectedVersion)
		if err != nil {
			return fmt.Errorf("local deletion transaction: %w", err)
		}
		if !moved {
			break
		}
	}
	for i := 0; i < 32; i++ {
		job, err := d.store.ClaimProviderErasure(ctx)
		if err != nil {
			return fmt.Errorf("claim deletion provider job: %w", err)
		}
		if job == nil {
			break
		}
		provider, ok := d.providers[job.ProviderRef]
		if !ok {
			if err := d.store.FailProviderErasure(ctx, *job, "PROVIDER_NOT_CONFIGURED"); err != nil {
				return err
			}
			continue
		}
		signed, err := provider.erase(ctx, *job)
		if err != nil {
			if recordErr := d.store.FailProviderErasure(ctx, *job, "HTTP_UNAVAILABLE"); recordErr != nil {
				return recordErr
			}
			continue
		}
		if err = d.store.SaveProviderErasure(ctx, *job, signed, provider.key); err != nil {
			if recordErr := d.store.FailProviderErasure(ctx, *job, "INVALID_PROVIDER_RECEIPT"); recordErr != nil {
				return recordErr
			}
		}
	}
	for i := 0; i < 16; i++ {
		done, err := d.store.FinalizeVerifiedDeletion(ctx)
		if err != nil {
			return fmt.Errorf("finalize verified deletion: %w", err)
		}
		if !done {
			break
		}
	}
	return nil
}
