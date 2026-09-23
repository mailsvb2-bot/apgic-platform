package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Capability string

var ErrInvalidRequest = errors.New("invalid connector request")

type Request struct {
	IdempotencyKey string
	SubjectID      string
	Payload        json.RawMessage
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.SubjectID) == "" {
		return ErrInvalidRequest
	}
	return nil
}

type Result struct {
	ProviderReference string
	Outcome           string
	RawEvidence       json.RawMessage
}

type Provider interface {
	Name() string
	Capabilities() []Capability
	Execute(ctx context.Context, capability Capability, request Request) (Result, error)
}
