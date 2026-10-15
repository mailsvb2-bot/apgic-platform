package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// FinanceOperationsStore backs the finance-operations discrepancy workflow.
// Operator capability is an explicit append-only grant provisioned outside
// the API; there is no self-service path to obtain it.
type FinanceOperationsStore interface {
	audit.Appender
	ActiveFinanceOperatorGrant(context.Context, string, time.Time) (string, bool, error)
	ListOpenFinanceDiscrepancies(context.Context, int) ([]finance.OpenDiscrepancy, error)
	ResolveFinanceDiscrepancy(context.Context, finance.DiscrepancyResolutionRequest) (finance.DiscrepancyResolution, error)
}

const (
	financeOpsTenant          = "apgic/finance-operations"
	financeOpsActionList      = "finance.discrepancy.list"
	financeOpsActionResolve   = "finance.discrepancy.resolve"
	financeOpsPolicyVersion   = "authz-policy-v1"
	financeOpsMaxRequestBytes = 16 * 1024
)

type financeOperatorContext struct {
	identityID string
	sessionRef string
	grantID    string
	auditID    string
}

// authorizeFinanceOperator binds the trusted session, evaluates the audited
// authz decision and writes the error response itself when not allowed.
func authorizeFinanceOperator(
	w http.ResponseWriter,
	r *http.Request,
	store FinanceOperationsStore,
	sessions *clientSessionManager,
	stepUp *stepUpManager,
	sessionConfigErr error,
	now func() time.Time,
	action string,
	risk authz.Risk,
	resourceID string,
) (financeOperatorContext, bool) {
	var op financeOperatorContext
	if store == nil || sessions == nil || stepUp == nil || sessionConfigErr != nil {
		writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_OPERATIONS_UNAVAILABLE", "Финансовые операции временно недоступны.", true, nil)
		return op, false
	}
	identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
	if !ok {
		return op, false
	}
	sessionIdentityID, sessionRef, err := sessions.identityAndReferenceFromRequest(r)
	if err != nil || sessionIdentityID != identityID {
		writeDemandError(w, r, http.StatusUnauthorized, "AUTH_SESSION_INVALID", "Сессия недействительна.", false, nil)
		return op, false
	}
	at := now().UTC()
	grantID, granted, err := store.ActiveFinanceOperatorGrant(r.Context(), identityID, at)
	if err != nil {
		writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_STORE_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
		return op, false
	}
	auditID, err := persistentid.New()
	if err != nil {
		writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_AUDIT_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
		return op, false
	}
	correlation := strings.TrimSpace(r.Header.Get("X-Correlation-Id"))
	if correlation == "" {
		correlation = "authz:" + auditID
	}
	principal := authz.Principal{ID: identityID, TenantID: financeOpsTenant, SessionRef: sessionRef, Permissions: map[string]struct{}{}}
	if granted {
		principal.Permissions[financeOpsActionList] = struct{}{}
		principal.Permissions[financeOpsActionResolve] = struct{}{}
	}
	if evidence := stepUp.evidenceFromRequest(r, identityID, sessionRef); evidence != nil {
		issued := evidence.IssuedAt
		principal.StepUpAt, principal.StepUpMethod = &issued, evidence.Method
	}
	result, err := (authz.Evaluator{PolicyVersion: financeOpsPolicyVersion, Appender: store}).Authorize(authz.Input{
		Principal:     principal,
		Resource:      authz.ResourceRef{ID: resourceID, TenantID: financeOpsTenant},
		Action:        action,
		Risk:          risk,
		Now:           at,
		MaxStepUpAge:  stepUpTTL,
		CorrelationID: correlation,
		AuditRecordID: auditID,
	})
	if err != nil {
		writeDemandError(w, r, http.StatusServiceUnavailable, "AUTH_AUDIT_UNAVAILABLE", "Проверка доступа временно недоступна.", true, nil)
		return op, false
	}
	if result.Decision == authz.StepUpRequired {
		writeDemandError(w, r, http.StatusPreconditionRequired, result.ReasonCode, "Для этого действия требуется повторное подтверждение личности.", false, []string{result.ReasonCode})
		return op, false
	}
	if result.Decision != authz.Allow {
		writeDemandError(w, r, http.StatusForbidden, result.ReasonCode, "Доступ к финансовым операциям запрещён.", false, []string{result.ReasonCode})
		return op, false
	}
	return financeOperatorContext{identityID: identityID, sessionRef: sessionRef, grantID: grantID, auditID: auditID}, true
}

func registerFinanceOperations(
	mux *http.ServeMux,
	store FinanceOperationsStore,
	sessions *clientSessionManager,
	stepUp *stepUpManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("GET /v1/finance/operations/discrepancies", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeFinanceOperator(w, r, store, sessions, stepUp, sessionConfigErr, now,
			financeOpsActionList, authz.RiskNormal, "discrepancies"); !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := store.ListOpenFinanceDiscrepancies(r.Context(), limit)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_DISCREPANCIES_UNAVAILABLE", "Список расхождений временно недоступен.", true, nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("POST /v1/finance/operations/discrepancies/{discrepancyID}/resolutions", func(w http.ResponseWriter, r *http.Request) {
		discrepancyID := strings.TrimSpace(r.PathValue("discrepancyID"))
		op, ok := authorizeFinanceOperator(w, r, store, sessions, stepUp, sessionConfigErr, now,
			financeOpsActionResolve, authz.RiskHigh, "discrepancy/"+discrepancyID)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, financeOpsMaxRequestBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var body finance.DiscrepancyResolutionRequest
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "FINANCE_RESOLUTION_INVALID", "Решение по расхождению заполнено неверно.", false, nil)
			return
		}
		body.DiscrepancyID = discrepancyID
		body.OperatorID, body.OperatorGrantID = op.identityID, op.grantID
		body.StepUpSessionRef, body.AuditRef = op.sessionRef, "audit/"+op.auditID
		body.ResolvedAt = now().UTC()
		resolution, err := store.ResolveFinanceDiscrepancy(r.Context(), body)
		switch {
		case err == nil:
			status := http.StatusCreated
			if resolution.Replayed {
				status = http.StatusOK
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, status, resolution)
		case errors.Is(err, finance.ErrDiscrepancyNotFound):
			writeDemandError(w, r, http.StatusNotFound, "FINANCE_DISCREPANCY_NOT_FOUND", "Расхождение не найдено.", false, nil)
		case errors.Is(err, finance.ErrOperatorNotGranted):
			writeDemandError(w, r, http.StatusForbidden, "AUTH_PERMISSION_DENIED", "Доступ к финансовым операциям запрещён.", false, nil)
		case errors.Is(err, finance.ErrDiscrepancyAlreadyResolved):
			writeDemandError(w, r, http.StatusConflict, "FINANCE_DISCREPANCY_ALREADY_RESOLVED", "Расхождение уже закрыто; история не изменена.", false, nil)
		case errors.Is(err, finance.ErrResolutionConflict):
			writeDemandError(w, r, http.StatusConflict, "FINANCE_RESOLUTION_IDEMPOTENCY_CONFLICT", "Ключ идемпотентности уже использован для другого решения.", false, nil)
		case errors.Is(err, finance.ErrResolutionInvalid):
			writeDemandError(w, r, http.StatusUnprocessableEntity, "FINANCE_RESOLUTION_INVALID", "Решение по расхождению не прошло проверку доказательств.", false, nil)
		default:
			writeDemandError(w, r, http.StatusServiceUnavailable, "FINANCE_RESOLUTION_UNAVAILABLE", "Финансовые операции временно недоступны.", true, nil)
		}
	})
}
