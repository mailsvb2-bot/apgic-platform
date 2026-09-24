package privacy

import (
	"errors"
	"strings"
	"time"
)

type DeletionSource string

const (
	DeletionSourceWeb     DeletionSource = "WEB"
	DeletionSourceIOS     DeletionSource = "IOS"
	DeletionSourceAndroid DeletionSource = "ANDROID"
)

type DeletionState string

const (
	DeletionRequested                 DeletionState = "REQUESTED"
	DeletionIdentityReconfirmed       DeletionState = "IDENTITY_RECONFIRMED"
	DeletionRetentionClassified       DeletionState = "RETENTION_CLASSIFIED"
	DeletionProviderErasurePending    DeletionState = "PROVIDER_ERASURE_PENDING"
	DeletionWaitingLegalHoldExpiry    DeletionState = "WAITING_FOR_LEGAL_HOLD_EXPIRY"
	DeletionPartiallyRetainedReason   DeletionState = "PARTIALLY_RETAINED_WITH_REASON"
	DeletionCompleted                 DeletionState = "COMPLETED"
)

type RetentionDisposition string

const (
	RetentionErase     RetentionDisposition = "ERASE"
	RetentionAnonymize RetentionDisposition = "ANONYMIZE"
	RetentionRetain    RetentionDisposition = "RETAIN_WITH_REASON"
)

type ErasureJobState string

const (
	ErasurePending   ErasureJobState = "PENDING"
	ErasureSucceeded ErasureJobState = "SUCCEEDED"
)

var (
	ErrInvalidDeletionRequest = errors.New("invalid delete account request")
	ErrInvalidDeletionState   = errors.New("invalid delete account state transition")
	ErrRetentionReasonRequired = errors.New("retained data requires explicit reason")
	ErrErasureIncomplete      = errors.New("provider erasure is incomplete")
)

type RetentionItem struct {
	DataClass    string
	Disposition  RetentionDisposition
	Reason       string
	LegalHoldRef string
}

type ProviderErasureJob struct {
	ProviderRef string
	State       ErasureJobState
	EvidenceRef string
}

type DeleteAccountRequest struct {
	ID             string
	IdentityID     string
	Source         DeletionSource
	State          DeletionState
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RetentionItems []RetentionItem
	ProviderJobs   []ProviderErasureJob
}

func NewDeleteAccountRequest(id, identityID string, source DeletionSource, now time.Time) (*DeleteAccountRequest, error) {
	if strings.TrimSpace(id) == "" ||
		strings.TrimSpace(identityID) == "" ||
		!validDeletionSource(source) ||
		now.IsZero() {
		return nil, ErrInvalidDeletionRequest
	}
	return &DeleteAccountRequest{
		ID:         id,
		IdentityID: identityID,
		Source:     source,
		State:      DeletionRequested,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (r *DeleteAccountRequest) ReconfirmIdentity(now time.Time) error {
	if r.State != DeletionRequested || now.IsZero() {
		return ErrInvalidDeletionState
	}
	r.State = DeletionIdentityReconfirmed
	r.UpdatedAt = now
	return nil
}

func (r *DeleteAccountRequest) ClassifyRetention(items []RetentionItem, now time.Time) error {
	if r.State != DeletionIdentityReconfirmed || len(items) == 0 || now.IsZero() {
		return ErrInvalidDeletionState
	}
	for _, item := range items {
		if strings.TrimSpace(item.DataClass) == "" || !validRetentionDisposition(item.Disposition) {
			return ErrInvalidDeletionRequest
		}
		if item.Disposition == RetentionRetain && strings.TrimSpace(item.Reason) == "" {
			return ErrRetentionReasonRequired
		}
	}
	r.RetentionItems = append([]RetentionItem(nil), items...)
	r.State = DeletionRetentionClassified
	r.UpdatedAt = now
	return nil
}

func (r *DeleteAccountRequest) BeginProviderErasure(jobs []ProviderErasureJob, now time.Time) error {
	if r.State != DeletionRetentionClassified || now.IsZero() {
		return ErrInvalidDeletionState
	}
	for _, job := range jobs {
		if strings.TrimSpace(job.ProviderRef) == "" || job.State != ErasurePending {
			return ErrInvalidDeletionRequest
		}
	}
	r.ProviderJobs = append([]ProviderErasureJob(nil), jobs...)
	r.State = DeletionProviderErasurePending
	r.UpdatedAt = now
	return nil
}

func (r *DeleteAccountRequest) MarkProviderErased(providerRef, evidenceRef string, now time.Time) error {
	if r.State != DeletionProviderErasurePending ||
		strings.TrimSpace(providerRef) == "" ||
		strings.TrimSpace(evidenceRef) == "" ||
		now.IsZero() {
		return ErrInvalidDeletionState
	}
	for i := range r.ProviderJobs {
		if r.ProviderJobs[i].ProviderRef == providerRef {
			r.ProviderJobs[i].State = ErasureSucceeded
			r.ProviderJobs[i].EvidenceRef = evidenceRef
			r.UpdatedAt = now
			return nil
		}
	}
	return ErrInvalidDeletionRequest
}

func (r *DeleteAccountRequest) Complete(now time.Time) error {
	if r.State != DeletionProviderErasurePending || now.IsZero() {
		return ErrInvalidDeletionState
	}
	for _, job := range r.ProviderJobs {
		if job.State != ErasureSucceeded || strings.TrimSpace(job.EvidenceRef) == "" {
			return ErrErasureIncomplete
		}
	}

	retained := false
	legalHold := false
	for _, item := range r.RetentionItems {
		if item.Disposition == RetentionRetain {
			retained = true
		}
		if strings.TrimSpace(item.LegalHoldRef) != "" {
			legalHold = true
		}
	}
	switch {
	case legalHold:
		r.State = DeletionWaitingLegalHoldExpiry
	case retained:
		r.State = DeletionPartiallyRetainedReason
	default:
		r.State = DeletionCompleted
	}
	r.UpdatedAt = now
	return nil
}

func validDeletionSource(source DeletionSource) bool {
	switch source {
	case DeletionSourceWeb, DeletionSourceIOS, DeletionSourceAndroid:
		return true
	default:
		return false
	}
}

func validRetentionDisposition(disposition RetentionDisposition) bool {
	switch disposition {
	case RetentionErase, RetentionAnonymize, RetentionRetain:
		return true
	default:
		return false
	}
}
