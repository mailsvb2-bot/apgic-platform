package authz

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
)

var (
	ErrAuditEvidenceRequired = errors.New("authorization audit evidence metadata is required")
	ErrAuditUnavailable      = errors.New("authorization audit evidence could not be persisted")
)

type Evaluator struct {
	PolicyVersion string
	Appender      audit.Appender
}

type decisionSnapshot struct {
	Decision   Decision `json:"decision"`
	ReasonCode string   `json:"reason_code"`
	Action     string   `json:"action"`
	Risk       Risk     `json:"risk"`
}

func (e Evaluator) Authorize(in Input) (Result, error) {
	result := Authorize(in)

	if strings.TrimSpace(e.PolicyVersion) == "" ||
		e.Appender == nil ||
		strings.TrimSpace(in.CorrelationID) == "" ||
		strings.TrimSpace(in.AuditRecordID) == "" {
		return Result{Decision: Deny, ReasonCode: "AUTH_AUDIT_EVIDENCE_REQUIRED"}, ErrAuditEvidenceRequired
	}

	actorID := strings.TrimSpace(in.Principal.ID)
	if actorID == "" {
		actorID = "anonymous"
	}
	scope := strings.TrimSpace(in.Principal.TenantID)
	if scope == "" {
		scope = "unauthenticated"
	}
	resourceRef := fmt.Sprintf(
		"tenant/%s/resource/%s",
		strings.TrimSpace(in.Resource.TenantID),
		strings.TrimSpace(in.Resource.ID),
	)

	state, err := json.Marshal(decisionSnapshot{
		Decision:   result.Decision,
		ReasonCode: result.ReasonCode,
		Action:     in.Action,
		Risk:       in.Risk,
	})
	if err != nil {
		return Result{Decision: Deny, ReasonCode: "AUTH_AUDIT_UNAVAILABLE"}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}

	record, err := audit.New(audit.Record{
		ID:            in.AuditRecordID,
		ActorID:       actorID,
		Action:        "authorization.decision",
		Scope:         scope,
		ResourceRef:   resourceRef,
		NewState:      state,
		Reason:        result.ReasonCode,
		PolicyVersion: e.PolicyVersion,
		OccurredAt:    in.Now,
		CorrelationID: in.CorrelationID,
	})
	if err != nil {
		return Result{Decision: Deny, ReasonCode: "AUTH_AUDIT_UNAVAILABLE"}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	if err := e.Appender.Append(record); err != nil {
		return Result{Decision: Deny, ReasonCode: "AUTH_AUDIT_UNAVAILABLE"}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}

	return result, nil
}
