package refunds

import (
	"errors"
	"strings"
	"time"
)

type State string

const (
	StateRequested      State = "REQUESTED"
	StateSent           State = "SENT"
	StatePending        State = "PENDING"
	StateSucceeded      State = "SUCCEEDED"
	StateFailedTerminal State = "FAILED_TERMINAL"
	StateAmbiguous      State = "AMBIGUOUS"
)

const ExternalExecutionOwner = "EXTERNAL_PROVIDER"

var (
	ErrInvalidRefund    = errors.New("invalid refund request")
	ErrTransitionDenied = errors.New("refund transition denied")
	ErrProviderMismatch = errors.New("refund must use original payment provider")
)

type Request struct {
	ID                       string
	BookingID                string
	OrderID                  string
	OriginalPaymentAttemptID string
	OriginalProviderID       string
	ProviderID               string
	AmountMinor              int64
	Currency                 string
	PolicyVersion            string
	PolicyDecision           string
	ReasonCode               string
	IdempotencyKey           string
	ExecutionOwner           string
	State                    State
	ProviderReference        string
	ProviderEvidenceRef      string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func New(request Request) (Request, error) {
	request.Currency = strings.ToUpper(request.Currency)
	if strings.TrimSpace(request.ID) == "" ||
		strings.TrimSpace(request.BookingID) == "" ||
		strings.TrimSpace(request.OrderID) == "" ||
		strings.TrimSpace(request.OriginalPaymentAttemptID) == "" ||
		strings.TrimSpace(request.OriginalProviderID) == "" ||
		strings.TrimSpace(request.ProviderID) == "" ||
		request.ProviderID != request.OriginalProviderID ||
		request.AmountMinor <= 0 ||
		len(request.Currency) != 3 ||
		strings.TrimSpace(request.PolicyVersion) == "" ||
		request.PolicyDecision != "ALLOW" ||
		strings.TrimSpace(request.ReasonCode) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" ||
		request.ExecutionOwner != ExternalExecutionOwner ||
		request.CreatedAt.IsZero() {
		return Request{}, ErrInvalidRefund
	}
	if request.UpdatedAt.IsZero() {
		request.UpdatedAt = request.CreatedAt
	}
	if request.UpdatedAt.Before(request.CreatedAt) {
		return Request{}, ErrInvalidRefund
	}
	request.State = StateRequested
	request.ProviderReference = ""
	request.ProviderEvidenceRef = ""
	return request, nil
}

func (r *Request) Transition(to State, now time.Time, providerReference, providerEvidenceRef string) error {
	if now.IsZero() || now.Before(r.UpdatedAt) {
		return ErrTransitionDenied
	}
	if !allowedTransition(r.State, to) {
		return ErrTransitionDenied
	}
	if to == StateSent && strings.TrimSpace(providerReference) == "" {
		return ErrTransitionDenied
	}
	if (to == StateSucceeded || to == StateFailedTerminal) && strings.TrimSpace(providerEvidenceRef) == "" {
		return ErrTransitionDenied
	}
	if r.ProviderReference != "" && providerReference != "" && providerReference != r.ProviderReference {
		return ErrTransitionDenied
	}
	if r.ProviderEvidenceRef != "" && providerEvidenceRef != "" && providerEvidenceRef != r.ProviderEvidenceRef {
		return ErrTransitionDenied
	}
	if providerReference != "" {
		r.ProviderReference = providerReference
	}
	if providerEvidenceRef != "" {
		r.ProviderEvidenceRef = providerEvidenceRef
	}
	r.State = to
	r.UpdatedAt = now
	return nil
}

func (r Request) ValidateExecutionProvider(providerID string) error {
	if strings.TrimSpace(providerID) == "" || providerID != r.OriginalProviderID || providerID != r.ProviderID {
		return ErrProviderMismatch
	}
	return nil
}

func allowedTransition(from, to State) bool {
	switch from {
	case StateRequested:
		return to == StateSent || to == StateFailedTerminal
	case StateSent:
		return to == StatePending || to == StateSucceeded || to == StateFailedTerminal || to == StateAmbiguous
	case StatePending:
		return to == StateSucceeded || to == StateFailedTerminal || to == StateAmbiguous
	case StateAmbiguous:
		return to == StateSucceeded || to == StateFailedTerminal
	default:
		return false
	}
}
