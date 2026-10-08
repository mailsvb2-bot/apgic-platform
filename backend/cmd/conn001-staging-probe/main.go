package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

type evidence struct {
	SchemaVersion              string `json:"schema_version"`
	EvidenceType               string `json:"evidence_type"`
	CandidateSHA               string `json:"candidate_sha"`
	ObservedAt                 string `json:"observed_at"`
	Environment                string `json:"environment"`
	ConnectorCapability        string `json:"connector_capability"`
	ConnectorStatus            string `json:"connector_status"`
	IdentityPersisted          bool   `json:"identity_persisted"`
	BookingHoldPersisted       bool   `json:"booking_hold_persisted"`
	BookingState               string `json:"booking_state"`
	LedgerEffectCount          int    `json:"ledger_effect_count"`
	ProviderExecutionClaimed   bool   `json:"provider_execution_claimed"`
	CanonicalTruthProviderOwned bool  `json:"canonical_truth_provider_owned"`
	BusinessTruthSurvived      bool   `json:"business_truth_survived"`
	ProductionEvidence         bool   `json:"production_evidence"`
}

func (e evidence) validate() error {
	if strings.TrimSpace(e.CandidateSHA) == "" {
		return errors.New("candidate SHA is required")
	}
	if e.Environment != "STAGING" {
		return fmt.Errorf("environment=%q want STAGING", e.Environment)
	}
	if e.ConnectorCapability != "COMMUNICATION_PROVIDER" {
		return fmt.Errorf("connector capability=%q want COMMUNICATION_PROVIDER", e.ConnectorCapability)
	}
	if e.ConnectorStatus != "DISABLED" {
		return fmt.Errorf("connector status=%q want DISABLED", e.ConnectorStatus)
	}
	if !e.IdentityPersisted || !e.BookingHoldPersisted || e.BookingState != "HELD" {
		return errors.New("canonical identity/booking truth did not survive provider unavailability")
	}
	if e.LedgerEffectCount != 1 {
		return fmt.Errorf("ledger effects=%d want 1", e.LedgerEffectCount)
	}
	if e.ProviderExecutionClaimed {
		return errors.New("probe must not fabricate provider execution")
	}
	if e.CanonicalTruthProviderOwned {
		return errors.New("canonical truth must remain APGIC-owned")
	}
	if !e.BusinessTruthSurvived {
		return errors.New("provider-neutral business truth proof incomplete")
	}
	if e.ProductionEvidence {
		return errors.New("staging probe must not claim production evidence")
	}
	return nil
}

func runProbe(ctx context.Context, databaseURL, candidateSHA string, now time.Time) (evidence, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	candidateSHA = strings.TrimSpace(candidateSHA)
	if databaseURL == "" {
		return evidence{}, errors.New("APGIC_DATABASE_URL is required")
	}
	if candidateSHA == "" {
		return evidence{}, errors.New("APGIC_COMMIT_SHA is required")
	}
	now = now.UTC().Truncate(time.Second)

	store, err := runtimepostgres.Open(ctx, databaseURL)
	if err != nil {
		return evidence{}, err
	}
	defer store.Close()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return evidence{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return evidence{}, fmt.Errorf("probe database ping: %w", err)
	}

	connectorID, err := persistentid.New()
	if err != nil {
		return evidence{}, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO connector_instances (
			id, capability_class, provider_kind, status, config_ref
		) VALUES ($1::uuid, 'COMMUNICATION_PROVIDER', 'staging-unavailable-probe', 'DISABLED', $2)
	`, connectorID, "secretref://connector/"+connectorID); err != nil {
		return evidence{}, fmt.Errorf("persist disabled communication connector: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_instances WHERE id = $1::uuid`, connectorID)
	}()

	service, err := demand.NewConformanceServiceWithStores(func() time.Time { return now }, store, store)
	if err != nil {
		return evidence{}, err
	}
	intent, err := service.CreateIntent("CONN-001 staging provider-neutral continuity probe")
	if err != nil {
		return evidence{}, fmt.Errorf("create intent with communication provider disabled: %w", err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		return evidence{}, fmt.Errorf("confirm intent with communication provider disabled: %w", err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		return evidence{}, fmt.Errorf("discover slots with communication provider disabled: slots=%d err=%w", len(slots), err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		return evidence{}, fmt.Errorf("acquire booking hold with communication provider disabled: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `
			UPDATE booking_holds
			   SET state = 'RELEASED',
			       updated_at = $2
			 WHERE id = $1::uuid
			   AND state = 'ACTIVE'
		`, hold.ID, time.Now().UTC())
	}()

	ledgerID, err := persistentid.New()
	if err != nil {
		return evidence{}, err
	}
	economicEventRef := "conn001-staging-proof/" + connectorID
	if _, err := store.AppendLedgerEntry(ledger.Entry{
		ID:                  ledgerID,
		DebitAccountRef:     "external-payer/" + intent.ClientIdentityID,
		CreditAccountRef:    "platform-evidence/conn001-staging",
		AmountMinor:         1,
		Currency:            "RUB",
		ProviderEvidenceRef: "provider-evidence://conn001/staging/non-custodial",
		EconomicEventRef:    economicEventRef,
		CorrelationID:       "conn001-staging-" + connectorID,
		OccurredAt:          now,
	}); err != nil {
		return evidence{}, fmt.Errorf("append ledger evidence with communication provider disabled: %w", err)
	}

	var identityCount, ledgerCount int
	var holdState, connectorStatus string
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM identities WHERE id = $1::uuid`,
		intent.ClientIdentityID,
	).Scan(&identityCount); err != nil {
		return evidence{}, err
	}
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM booking_holds WHERE id = $1::uuid`,
		hold.ID,
	).Scan(&holdState); err != nil {
		return evidence{}, err
	}
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM ledger_entries WHERE economic_event_ref = $1`,
		economicEventRef,
	).Scan(&ledgerCount); err != nil {
		return evidence{}, err
	}
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM connector_instances WHERE id = $1::uuid`,
		connectorID,
	).Scan(&connectorStatus); err != nil {
		return evidence{}, err
	}

	out := evidence{
		SchemaVersion:               "conn001-staging-core-continuity-v1",
		EvidenceType:                "STAGING_PROVIDER_NEUTRAL_CORE_PROOF",
		CandidateSHA:                candidateSHA,
		ObservedAt:                  now.Format(time.RFC3339),
		Environment:                 "STAGING",
		ConnectorCapability:         "COMMUNICATION_PROVIDER",
		ConnectorStatus:             connectorStatus,
		IdentityPersisted:           identityCount == 1,
		BookingHoldPersisted:        holdState == "ACTIVE",
		BookingState:                hold.BookingState,
		LedgerEffectCount:           ledgerCount,
		ProviderExecutionClaimed:    false,
		CanonicalTruthProviderOwned: false,
		BusinessTruthSurvived:       identityCount == 1 && holdState == "ACTIVE" && hold.BookingState == "HELD" && ledgerCount == 1 && connectorStatus == "DISABLED",
		ProductionEvidence:          false,
	}
	if err := out.validate(); err != nil {
		return out, err
	}
	return out, nil
}

func main() {
	if strings.ToUpper(strings.TrimSpace(os.Getenv("APGIC_ENVIRONMENT"))) != "STAGING" {
		fmt.Fprintln(os.Stderr, "CONN-001 staging probe requires APGIC_ENVIRONMENT=STAGING")
		os.Exit(1)
	}
	out, err := runProbe(
		context.Background(),
		os.Getenv("APGIC_DATABASE_URL"),
		os.Getenv("APGIC_COMMIT_SHA"),
		time.Now(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
