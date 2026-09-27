package runtimepostgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/refunds"
)

const journeyWriteTimeout = 3 * time.Second

func catalogSpecialistIdentityUUID(publicRef string) (string, error) {
	return persistentid.FromRef("catalog-specialist-identity", publicRef)
}

func catalogSlotUUID(publicRef string) (string, error) {
	return persistentid.FromRef("catalog-slot", publicRef)
}

func (c *Checker) BootstrapCatalog(slots []demand.Slot) ([]demand.Slot, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin catalog bootstrap: %w", err)
	}
	defer tx.Rollback()

	result := make([]demand.Slot, 0, len(slots))
	for _, slot := range slots {
		specialistIdentityID, err := catalogSpecialistIdentityUUID(slot.SpecialistID)
		if err != nil {
			return nil, err
		}
		slotID, err := catalogSlotUUID(slot.ID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING`,
			specialistIdentityID,
		); err != nil {
			return nil, fmt.Errorf("bootstrap specialist identity %s: %w", slot.SpecialistID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO identity_roles (identity_id, role_code)
			 VALUES ($1::uuid, 'SPECIALIST')
			 ON CONFLICT (identity_id, role_code) DO NOTHING`,
			specialistIdentityID,
		); err != nil {
			return nil, fmt.Errorf("bootstrap specialist role %s: %w", slot.SpecialistID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO booking_slots (
				id, specialist_identity_id, tenant_scope, starts_at, ends_at, exclusive
			) VALUES ($1::uuid, $2::uuid, 'catalog/conformance', $3, $4, $5)
			ON CONFLICT (id) DO NOTHING`,
			slotID, specialistIdentityID, slot.StartsAt, slot.EndsAt, slot.Exclusive,
		); err != nil {
			return nil, fmt.Errorf("bootstrap slot %s: %w", slot.ID, err)
		}
		canonical := slot
		if err := tx.QueryRowContext(ctx,
			`SELECT starts_at, ends_at, exclusive
			 FROM booking_slots WHERE id = $1::uuid`,
			slotID,
		).Scan(&canonical.StartsAt, &canonical.EndsAt, &canonical.Exclusive); err != nil {
			return nil, fmt.Errorf("read canonical slot %s: %w", slot.ID, err)
		}
		result = append(result, canonical)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit catalog bootstrap: %w", err)
	}
	return result, nil
}

func (c *Checker) LoadJourney(slots []demand.Slot) (demand.JourneySnapshot, error) {
	if c == nil || c.db == nil {
		return demand.JourneySnapshot{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()

	slotRefs := make(map[string]string, len(slots))
	for _, slot := range slots {
		id, err := catalogSlotUUID(slot.ID)
		if err != nil {
			return demand.JourneySnapshot{}, err
		}
		slotRefs[id] = slot.ID
	}

	var snapshot demand.JourneySnapshot
	rows, err := c.db.QueryContext(ctx,
		`SELECT id::text, identity_id::text, free_text,
		        to_json(topics)::text, to_json(goals)::text, context::text, state
		 FROM help_intents
		 WHERE identity_id IS NOT NULL
		 ORDER BY created_at, id`)
	if err != nil {
		return snapshot, fmt.Errorf("load help intents: %w", err)
	}
	for rows.Next() {
		var intent demand.Intent
		var topicsJSON string
		var goalsJSON string
		var contextJSON string
		var status string
		if err := rows.Scan(
			&intent.ID, &intent.ClientIdentityID, &intent.FreeText,
			&topicsJSON, &goalsJSON, &contextJSON, &status,
		); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("scan help intent: %w", err)
		}
		if err := json.Unmarshal([]byte(topicsJSON), &intent.Topics); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("decode help intent topics: %w", err)
		}
		if err := json.Unmarshal([]byte(goalsJSON), &intent.Goals); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("decode help intent goals: %w", err)
		}
		if err := json.Unmarshal([]byte(contextJSON), &intent.Context); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("decode help intent context: %w", err)
		}
		intent.Status = marketplace.HelpIntentStatus(status)
		intent.CatalogMode = demand.CatalogModeConformance
		snapshot.Intents = append(snapshot.Intents, &intent)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("iterate help intents: %w", err)
	}
	rows.Close()

	rows, err = c.db.QueryContext(ctx,
		`SELECT h.id::text, h.slot_id::text, h.client_identity_id::text, h.state, h.expires_at
		 FROM booking_holds h
		 JOIN booking_slots s ON s.id = h.slot_id
		 WHERE s.tenant_scope = 'catalog/conformance'
		 ORDER BY h.created_at, h.id`)
	if err != nil {
		return snapshot, fmt.Errorf("load booking holds: %w", err)
	}
	for rows.Next() {
		var hold demand.Hold
		var slotUUID string
		if err := rows.Scan(&hold.ID, &slotUUID, &hold.ClientIdentityID, &hold.State, &hold.ExpiresAt); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("scan booking hold: %w", err)
		}
		publicSlot, ok := slotRefs[slotUUID]
		if !ok {
			rows.Close()
			return snapshot, fmt.Errorf("booking hold references unknown catalog slot %s", slotUUID)
		}
		hold.SlotID = publicSlot
		hold.BookingID, err = demandBookingIDForHold(hold.ID)
		if err != nil {
			rows.Close()
			return snapshot, err
		}
		switch hold.State {
		case "EXPIRED":
			hold.BookingState = booking.StateExpired
		default:
			hold.BookingState = booking.StateHeld
		}
		hold.ReasonCode = "BOOK_HOLD_ACQUIRED"
		snapshot.Holds = append(snapshot.Holds, &hold)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("iterate booking holds: %w", err)
	}
	rows.Close()

	rows, err = c.db.QueryContext(ctx,
		`SELECT b.id::text, b.slot_id::text, b.hold_id::text, b.client_identity_id::text,
		        b.state, b.hold_expires_at, b.starts_at, b.ends_at, b.updated_at
		 FROM bookings b
		 JOIN booking_slots s ON s.id = b.slot_id
		 WHERE s.tenant_scope = 'catalog/conformance'
		 ORDER BY b.created_at, b.id`)
	if err != nil {
		return snapshot, fmt.Errorf("load bookings: %w", err)
	}
	for rows.Next() {
		var booked booking.Booking
		var slotUUID string
		var state string
		if err := rows.Scan(
			&booked.ID, &slotUUID, &booked.HoldID, &booked.ClientIdentityID,
			&state, &booked.HoldExpiresAt, &booked.StartsAt, &booked.EndsAt, &booked.UpdatedAt,
		); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("scan booking: %w", err)
		}
		publicSlot, ok := slotRefs[slotUUID]
		if !ok {
			rows.Close()
			return snapshot, fmt.Errorf("booking references unknown catalog slot %s", slotUUID)
		}
		booked.SlotID = publicSlot
		booked.State = booking.State(state)
		snapshot.Bookings = append(snapshot.Bookings, &booked)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("iterate bookings: %w", err)
	}
	rows.Close()

	rows, err = c.db.QueryContext(ctx,
		`SELECT o.id::text, b.hold_id::text, b.id::text, b.state,
		        connector.provider_kind, decision.selected_method_code, decision.selected_rail_code,
		        o.amount_minor, o.currency, config.execution_owner,
		        o.payment_recipient_ref, o.platform_role
		 FROM orders o
		 JOIN bookings b ON b.id = o.booking_id
		 JOIN booking_slots s ON s.id = b.slot_id
		 JOIN LATERAL (
		   SELECT candidate.routing_decision_id
		   FROM payment_attempts candidate
		   WHERE candidate.order_id = o.id
		   ORDER BY candidate.created_at DESC, candidate.id DESC
		   LIMIT 1
		 ) attempt ON true
		 JOIN payment_routing_decisions decision ON decision.id = attempt.routing_decision_id
		 JOIN payment_provider_config_versions config ON config.id = decision.provider_config_id
		 JOIN connector_instances connector ON connector.id = config.provider_instance_id
		 WHERE s.tenant_scope = 'catalog/conformance'
		 ORDER BY o.captured_at, o.id`)
	if err != nil {
		return snapshot, fmt.Errorf("load checkout instructions: %w", err)
	}
	for rows.Next() {
		var instruction demand.CheckoutInstruction
		var bookingState string
		if err := rows.Scan(
			&instruction.OrderID,
			&instruction.HoldID,
			&instruction.BookingID,
			&bookingState,
			&instruction.ProviderID,
			&instruction.MethodCode,
			&instruction.RailCode,
			&instruction.AmountMinor,
			&instruction.Currency,
			&instruction.ExecutionOwner,
			&instruction.PaymentRecipientID,
			&instruction.PlatformRole,
		); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("scan checkout instruction: %w", err)
		}
		instruction.ID, err = persistentid.FromRef("checkout-instruction", instruction.OrderID)
		if err != nil {
			rows.Close()
			return snapshot, err
		}
		instruction.BookingState = booking.State(bookingState)
		instruction.APGICAcceptsFunds = false
		instruction.ReasonCode = payments.ReasonRouteSelected
		snapshot.Instructions = append(snapshot.Instructions, &instruction)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("iterate checkout instructions: %w", err)
	}
	rows.Close()

	instructionsByOrder := make(map[string]*demand.CheckoutInstruction, len(snapshot.Instructions))
	for _, instruction := range snapshot.Instructions {
		if instruction != nil {
			instructionsByOrder[instruction.OrderID] = instruction
		}
	}
	rows, err = c.db.QueryContext(ctx,
		`SELECT le.id::text, le.debit_account_ref, le.credit_account_ref,
		        le.amount_minor, le.currency, le.provider_evidence_ref,
		        le.economic_event_ref, le.correlation_id
		 FROM ledger_entries le
		 JOIN orders o
		   ON le.economic_event_ref = o.id::text
		   OR le.economic_event_ref = 'reversal:' || o.id::text
		 JOIN bookings b ON b.id = o.booking_id
		 JOIN booking_slots s ON s.id = b.slot_id
		 WHERE s.tenant_scope = 'catalog/conformance'
		 ORDER BY le.occurred_at, le.id`)
	if err != nil {
		return snapshot, fmt.Errorf("load journey ledger effects: %w", err)
	}
	capturesByOrder := make(map[string]*demand.PaymentEvidence)
	for rows.Next() {
		var ledgerID, debit, credit, currency, providerEvidenceRef, economicEventRef, correlationID string
		var amountMinor int64
		if err := rows.Scan(
			&ledgerID, &debit, &credit, &amountMinor, &currency,
			&providerEvidenceRef, &economicEventRef, &correlationID,
		); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("scan journey ledger effect: %w", err)
		}
		if strings.HasPrefix(economicEventRef, "reversal:") {
			orderID := strings.TrimPrefix(economicEventRef, "reversal:")
			instruction := instructionsByOrder[orderID]
			capture := capturesByOrder[orderID]
			if instruction == nil || capture == nil {
				rows.Close()
				return snapshot, fmt.Errorf("reversal references incomplete journey order %s", orderID)
			}
			refundID, err := persistentid.FromRef("refund-for-order", orderID)
			if err != nil {
				rows.Close()
				return snapshot, err
			}
			snapshot.Reversals = append(snapshot.Reversals, &demand.Cancellation{
				ID:                refundID,
				OrderID:           orderID,
				BookingID:         instruction.BookingID,
				BookingState:      booking.StateCancelled,
				RefundID:          refundID,
				RefundState:       refunds.StateSucceeded,
				ProviderID:        capture.ProviderID,
				ExecutionOwner:    refunds.ExternalExecutionOwner,
				OriginalLedgerID:  capture.LedgerEntryID,
				ReversalLedgerID:  ledgerID,
				AmountMinor:       amountMinor,
				Currency:          currency,
				APGICAcceptsFunds: false,
				APGICReturnsFunds: false,
			})
			continue
		}
		instruction := instructionsByOrder[economicEventRef]
		if instruction == nil {
			rows.Close()
			return snapshot, fmt.Errorf("capture references unknown journey order %s", economicEventRef)
		}
		providerEventID := providerEvidenceRef
		prefix := instruction.ProviderID + "/"
		if strings.HasPrefix(providerEvidenceRef, prefix) {
			providerEventID = strings.TrimPrefix(providerEvidenceRef, prefix)
		}
		evidenceID, err := persistentid.FromRef("payment-evidence", providerEvidenceRef)
		if err != nil {
			rows.Close()
			return snapshot, err
		}
		created := &demand.PaymentEvidence{
			ID:                evidenceID,
			OrderID:           economicEventRef,
			BookingID:         instruction.BookingID,
			BookingState:      booking.StateConfirmed,
			ProviderID:        instruction.ProviderID,
			ProviderEventID:   providerEventID,
			LedgerEntryID:     ledgerID,
			AmountMinor:       amountMinor,
			Currency:          currency,
			DebitAccountRef:   debit,
			CreditAccountRef:  credit,
			APGICAcceptsFunds: false,
		}
		capturesByOrder[economicEventRef] = created
		snapshot.Evidence = append(snapshot.Evidence, created)
		_ = correlationID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("iterate journey ledger effects: %w", err)
	}
	rows.Close()
	return snapshot, nil
}

func demandBookingIDForHold(holdID string) (string, error) {
	return persistentid.FromRef("booking-for-hold", holdID)
}

func (c *Checker) CreateIntent(intent *demand.Intent) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	if intent == nil {
		return errors.New("help intent is required")
	}
	contextJSON, err := json.Marshal(intent.Context)
	if err != nil {
		return fmt.Errorf("encode help intent context: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin help intent create: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING`,
		intent.ClientIdentityID,
	); err != nil {
		return fmt.Errorf("create client identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identity_roles (identity_id, role_code)
		 VALUES ($1::uuid, 'CLIENT')
		 ON CONFLICT (identity_id, role_code) DO NOTHING`,
		intent.ClientIdentityID,
	); err != nil {
		return fmt.Errorf("create client role: %w", err)
	}
	topics := intent.Topics
	if topics == nil {
		topics = []string{}
	}
	goals := intent.Goals
	if goals == nil {
		goals = []string{}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO help_intents (
			id, identity_id, free_text, topics, goals, context, state
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6::jsonb, $7
		)`,
		intent.ID,
		intent.ClientIdentityID,
		intent.FreeText,
		topics,
		goals,
		string(contextJSON),
		string(intent.Status),
	); err != nil {
		return fmt.Errorf("create help intent: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit help intent create: %w", err)
	}
	return nil
}

func (c *Checker) ConfirmIntent(intent *demand.Intent, confirmedAt time.Time) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	if intent == nil || confirmedAt.IsZero() {
		return errors.New("confirmed help intent and timestamp are required")
	}
	contextJSON, err := json.Marshal(intent.Context)
	if err != nil {
		return fmt.Errorf("encode confirmed intent context: %w", err)
	}
	topics := intent.Topics
	if topics == nil {
		topics = []string{}
	}
	goals := intent.Goals
	if goals == nil {
		goals = []string{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	result, err := c.db.ExecContext(ctx,
		`UPDATE help_intents
		 SET topics = $2,
		     goals = $3,
		     context = $4::jsonb,
		     state = 'CONFIRMED',
		     confirmed_at = $5,
		     updated_at = $5
		 WHERE id = $1::uuid
		   AND identity_id = $6::uuid`,
		intent.ID,
		topics,
		goals,
		string(contextJSON),
		confirmedAt,
		intent.ClientIdentityID,
	)
	if err != nil {
		return fmt.Errorf("confirm help intent: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm help intent rows: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("confirm help intent affected %d rows", affected)
	}
	return nil
}

func (c *Checker) AcquireHold(hold *demand.Hold, slot demand.Slot, now time.Time) (string, error) {
	if c == nil || c.db == nil {
		return "", errors.New("postgres checker is not initialized")
	}
	if hold == nil || now.IsZero() {
		return "", errors.New("hold and timestamp are required")
	}
	slotID, err := catalogSlotUUID(slot.ID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	var acquired bool
	var reason string
	if err := c.db.QueryRowContext(ctx,
		`SELECT acquired, reason_code
		 FROM apgic_acquire_slot_hold($1::uuid, $2::uuid, $3::uuid, $4, $5)`,
		hold.ID,
		slotID,
		hold.ClientIdentityID,
		hold.ExpiresAt,
		now,
	).Scan(&acquired, &reason); err != nil {
		return "", fmt.Errorf("acquire canonical slot hold: %w", err)
	}
	if !acquired {
		return reason, nil
	}
	return "BOOK_HOLD_ACQUIRED", nil
}

func (c *Checker) CreateCheckout(persistence demand.CheckoutPersistence) (string, error) {
	if c == nil || c.db == nil {
		return "", errors.New("postgres checker is not initialized")
	}
	booked := persistence.Booking
	instruction := persistence.Instruction
	order := persistence.Order
	legalSnapshot := persistence.LegalSnapshot
	if booked == nil || instruction == nil || persistence.DecidedAt.IsZero() ||
		order.ID == "" || persistence.RoutingPolicyVersion == "" {
		return "", errors.New("complete checkout persistence snapshot is required")
	}
	if order.ID != instruction.OrderID ||
		order.BookingID != booked.ID ||
		instruction.BookingID != booked.ID ||
		instruction.HoldID != booked.HoldID ||
		instruction.AmountMinor != order.AmountMinor ||
		instruction.Currency != order.Currency ||
		instruction.PaymentRecipientID != order.PaymentRecipientRef ||
		instruction.PlatformRole != order.PlatformRole {
		return "", errors.New("checkout persistence snapshot is inconsistent")
	}

	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin checkout create: %w", err)
	}
	defer tx.Rollback()

	var created bool
	var reason string
	if err := tx.QueryRowContext(ctx,
		`SELECT created, reason_code
		 FROM apgic_create_booking_from_hold($1::uuid, $2::uuid, $3)`,
		booked.ID,
		booked.HoldID,
		persistence.DecidedAt,
	).Scan(&created, &reason); err != nil {
		return "", fmt.Errorf("create canonical booking: %w", err)
	}
	if !created {
		return reason, nil
	}
	if booked.State != booking.StateHeld {
		result, err := tx.ExecContext(ctx,
			`UPDATE bookings
			 SET state = $2,
			     updated_at = $3
			 WHERE id = $1::uuid`,
			booked.ID,
			string(booked.State),
			booked.UpdatedAt,
		)
		if err != nil {
			return "", fmt.Errorf("advance canonical booking: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("advance canonical booking rows: %w", err)
		}
		if affected != 1 {
			return "", fmt.Errorf("advance canonical booking affected %d rows", affected)
		}
	}

	legalSnapshotID, err := persistentid.FromRef("legal-transaction", order.ID)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO legal_transaction_snapshots (
			id, transaction_ref, seller_or_service_provider_id, commercial_owner_id,
			payment_recipient_id, platform_role, fiscal_responsibility_id,
			refund_responsibility_id, payout_beneficiary_id, policy_version, occurred_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)`,
		legalSnapshotID,
		"order/"+order.ID,
		legalSnapshot.SellerOrServiceProviderID,
		legalSnapshot.CommercialOwnerID,
		legalSnapshot.PaymentRecipientID,
		legalSnapshot.PlatformRole,
		legalSnapshot.FiscalResponsibilityID,
		legalSnapshot.RefundResponsibilityID,
		legalSnapshot.PayoutBeneficiaryID,
		legalSnapshot.PolicyVersion,
		order.CapturedAt,
	); err != nil {
		return "", fmt.Errorf("persist legal transaction snapshot: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO orders (
			id, booking_id, offer_ref, price_source_ref, amount_minor, currency,
			commission_minor, pricing_policy_version, commission_policy_version,
			legal_snapshot_id, seller_ref, commercial_owner_ref, payment_recipient_ref,
			platform_role, fiscal_responsibility_ref, refund_responsibility_ref,
			payout_beneficiary_ref, captured_at
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9,
			$10::uuid, $11, $12, $13, $14, $15, $16, $17, $18
		)`,
		order.ID,
		order.BookingID,
		order.OfferRef,
		order.PriceSourceRef,
		order.AmountMinor,
		order.Currency,
		order.CommissionMinor,
		order.PricingPolicyVersion,
		order.CommissionPolicyVersion,
		legalSnapshotID,
		order.SellerRef,
		order.CommercialOwnerRef,
		order.PaymentRecipientRef,
		order.PlatformRole,
		order.FiscalResponsibilityRef,
		order.RefundResponsibilityRef,
		order.PayoutBeneficiaryRef,
		order.CapturedAt,
	); err != nil {
		return "", fmt.Errorf("persist immutable order: %w", err)
	}

	providerConfigRef := "conformance:" + instruction.ProviderID
	var providerInstanceID string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE((
			SELECT id::text
			FROM connector_instances
			WHERE capability_class = 'PAYMENT_PROVIDER'
			  AND provider_kind = $1
			  AND config_ref = $2
			LIMIT 1
		), '')`,
		instruction.ProviderID,
		providerConfigRef,
	).Scan(&providerInstanceID); err != nil {
		return "", fmt.Errorf("find payment connector: %w", err)
	}
	if providerInstanceID == "" {
		providerInstanceID, err = persistentid.FromRef("payment-connector", instruction.ProviderID)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO connector_instances (
				id, capability_class, provider_kind, status, config_ref
			) VALUES ($1::uuid, 'PAYMENT_PROVIDER', $2, 'ACTIVE', $3)`,
			providerInstanceID,
			instruction.ProviderID,
			providerConfigRef,
		); err != nil {
			return "", fmt.Errorf("persist payment connector: %w", err)
		}
	}
	var connectorCapability, connectorStatus string
	if err := tx.QueryRowContext(ctx,
		`SELECT capability_class, status
		 FROM connector_instances
		 WHERE id = $1::uuid`,
		providerInstanceID,
	).Scan(&connectorCapability, &connectorStatus); err != nil {
		return "", fmt.Errorf("read payment connector: %w", err)
	}
	if connectorCapability != "PAYMENT_PROVIDER" ||
		(connectorStatus != "ACTIVE" && connectorStatus != "DEGRADED") {
		return "", errors.New("payment connector is not routable")
	}

	var providerConfigID string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE((
			SELECT id::text
			FROM payment_provider_config_versions
			WHERE provider_instance_id = $1::uuid
			  AND config_version = $2
			LIMIT 1
		), '')`,
		providerInstanceID,
		persistence.RoutingPolicyVersion,
	).Scan(&providerConfigID); err != nil {
		return "", fmt.Errorf("find provider config: %w", err)
	}
	if providerConfigID == "" {
		providerConfigID, err = persistentid.FromRef(
			"payment-provider-config",
			providerInstanceID+":"+persistence.RoutingPolicyVersion,
		)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO payment_provider_config_versions (
				id, provider_instance_id, config_version, status, priority, manifest_version,
				certification_evidence_refs, jurisdiction_codes, currencies, method_codes,
				rail_codes, execution_owner, effective_from, routing_weight_bps,
				credential_version_ref
			) VALUES (
				$1::uuid, $2::uuid, $3, 'ACTIVE', 0, 'conformance-external-v1',
				ARRAY['conformance:not-production-psp'], ARRAY['RU'], ARRAY['RUB'],
				ARRAY['BANK_CARD','SBP'], ARRAY['BANK_TRANSFER_RAIL'], $4,
				$5, 10000, 'conformance:external-bank:v1'
			)`,
			providerConfigID,
			providerInstanceID,
			persistence.RoutingPolicyVersion,
			instruction.ExecutionOwner,
			persistence.DecidedAt.Add(-time.Second),
		); err != nil {
			return "", fmt.Errorf("persist provider config: %w", err)
		}
	}

	var healthSnapshotID, health, guardrail string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(id::text, ''), COALESCE(health, ''), COALESCE(guardrail_action, '')
		 FROM (
			SELECT id, health, guardrail_action
			FROM payment_provider_health_snapshots
			WHERE provider_config_id = $1::uuid
			  AND observed_at <= $2
			ORDER BY observed_at DESC, created_at DESC, id DESC
			LIMIT 1
		 ) latest
		 RIGHT JOIN (SELECT 1) sentinel ON true`,
		providerConfigID,
		persistence.DecidedAt,
	).Scan(&healthSnapshotID, &health, &guardrail); err != nil {
		return "", fmt.Errorf("find provider health snapshot: %w", err)
	}
	if healthSnapshotID == "" {
		healthSnapshotID, err = persistentid.FromRef(
			"payment-provider-health",
			providerConfigID+":baseline",
		)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO payment_provider_health_snapshots (
				id, provider_config_id, health, conversion_rate_bps, latency_p95_ms,
				provider_reported_fee_bps, reconciliation_pending_count,
				reconciliation_mismatch_count, guardrail_action, evidence_refs, observed_at
			) VALUES (
				$1::uuid, $2::uuid, 'HEALTHY', 10000, 0, 0, 0, 0,
				'ALLOW_NEW_ATTEMPTS', ARRAY['conformance:not-production-health'], $3
			)`,
			healthSnapshotID,
			providerConfigID,
			persistence.DecidedAt,
		); err != nil {
			return "", fmt.Errorf("persist provider health snapshot: %w", err)
		}
		health = "HEALTHY"
		guardrail = "ALLOW_NEW_ATTEMPTS"
	}
	if health == "UNAVAILABLE" || guardrail != "ALLOW_NEW_ATTEMPTS" {
		return "", errors.New("provider health blocks new payment attempts")
	}

	candidateEvidence, err := json.Marshal([]map[string]any{{
		"provider_id": instruction.ProviderID,
		"eligible":    true,
	}})
	if err != nil {
		return "", fmt.Errorf("encode routing candidates: %w", err)
	}
	healthEvidence, err := json.Marshal(map[string]any{
		"health":           health,
		"guardrail_action": guardrail,
	})
	if err != nil {
		return "", fmt.Errorf("encode routing health: %w", err)
	}
	routingDecisionID, err := persistentid.FromRef("payment-routing-decision", order.ID)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO payment_routing_decisions (
			id, order_id, policy_version, provider_config_id, jurisdiction_code,
			selected_method_code, selected_rail_code, candidate_evidence,
			health_snapshot, decided_at, health_snapshot_id
		) VALUES (
			$1::uuid, $2::uuid, $3, $4::uuid, 'RU',
			$5, $6, $7::jsonb, $8::jsonb, $9, $10::uuid
		)`,
		routingDecisionID,
		order.ID,
		persistence.RoutingPolicyVersion,
		providerConfigID,
		instruction.MethodCode,
		instruction.RailCode,
		string(candidateEvidence),
		string(healthEvidence),
		persistence.DecidedAt,
		healthSnapshotID,
	); err != nil {
		return "", fmt.Errorf("persist routing decision: %w", err)
	}

	paymentAttemptID, err := persistentid.FromRef("payment-attempt", order.ID)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO payment_attempts (
			id, order_id, routing_decision_id, idempotency_key, amount_minor,
			currency, state, created_at, updated_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, 'CREATED', $7, $7
		)`,
		paymentAttemptID,
		order.ID,
		routingDecisionID,
		instruction.HoldID+":"+instruction.MethodCode,
		instruction.AmountMinor,
		instruction.Currency,
		persistence.DecidedAt,
	); err != nil {
		return "", fmt.Errorf("persist payment attempt: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE payment_attempts
		 SET state = 'SENT',
		     updated_at = $2
		 WHERE id = $1::uuid`,
		paymentAttemptID,
		persistence.DecidedAt,
	); err != nil {
		return "", fmt.Errorf("mark payment attempt sent: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit durable checkout: %w", err)
	}
	return "BOOK_CREATED", nil
}

func (c *Checker) Expire(now time.Time) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	if now.IsZero() {
		return errors.New("expiry timestamp is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), journeyWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin journey expiry: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE booking_holds
		 SET state = 'EXPIRED',
		     updated_at = $1
		 WHERE state = 'ACTIVE'
		   AND expires_at <= $1`,
		now,
	); err != nil {
		return fmt.Errorf("expire canonical holds: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bookings
		 SET state = 'EXPIRED',
		     updated_at = $1
		 WHERE state IN ('HELD','PENDING_PAYMENT')
		   AND hold_expires_at <= $1`,
		now,
	); err != nil {
		return fmt.Errorf("expire canonical bookings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit journey expiry: %w", err)
	}
	return nil
}
