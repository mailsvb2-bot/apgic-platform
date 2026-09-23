package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidRequest = errors.New("connector request requires idempotency key and subject id")

type Request struct { IdempotencyKey string; SubjectID string; Payload json.RawMessage }
func (r Request) Validate() error {
	if strings.TrimSpace(r.IdempotencyKey)=="" || strings.TrimSpace(r.SubjectID)=="" { return ErrInvalidRequest }
	return nil
}
type Result struct { ProviderReference string; Outcome string; RawEvidence json.RawMessage }
type Provider interface {
	Kind() string
	Capabilities() []CapabilityClass
	Execute(context.Context, CapabilityClass, Request) (Result,error)
}
