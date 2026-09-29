package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestJourneyStoreReusesCanonicalClientIdentityAcrossIntents(t *testing.T) {
	databaseURL := os.Getenv("APGIC_JOURNEY_STORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("journey integration database not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store := &Checker{db: db}
	defer store.Close()

	identityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.CreateIntentForIdentity(identityID, "первый запрос")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntentForIdentity(identityID, "второй запрос")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.ClientIdentityID != identityID || second.ClientIdentityID != identityID {
		t.Fatalf("durable identity reuse failed: first=%#v second=%#v", first, second)
	}

	var identityCount, roleCount, intentCount int
	if err := store.db.QueryRow(
		"SELECT count(*) FROM identities WHERE id = $1::uuid",
		identityID,
	).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		"SELECT count(*) FROM identity_roles WHERE identity_id = $1::uuid AND role_code = 'CLIENT'",
		identityID,
	).Scan(&roleCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		"SELECT count(*) FROM help_intents WHERE identity_id = $1::uuid AND id IN ($2::uuid, $3::uuid)",
		identityID, first.ID, second.ID,
	).Scan(&intentCount); err != nil {
		t.Fatal(err)
	}
	if identityCount != 1 || roleCount != 1 || intentCount != 2 {
		t.Fatalf("canonical identity persistence identity=%d role=%d intents=%d", identityCount, roleCount, intentCount)
	}
}

func TestJourneyStoreSurvivesServiceRestart(t *testing.T) {
	databaseURL := os.Getenv("APGIC_JOURNEY_STORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("journey integration database not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store := &Checker{db: db}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Second)

	first, err := demand.NewConformanceServiceWithStores(func() time.Time { return now }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := demand.NewConformanceServiceWithStores(func() time.Time { return now }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := first.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := peer.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatalf("long-lived peer did not refresh new intent: %v", err)
	}
	slots, err := peer.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := first.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatalf("origin instance did not refresh peer confirmation: %v", err)
	}

	secondNow := now.Add(time.Minute)
	second, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	matches, _, err := second.Matches(intent.ID, "sleep")
	if err != nil || len(matches) == 0 {
		t.Fatalf("hydrated intent matches=%#v err=%v", matches, err)
	}
	replayedHold, err := second.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatalf("hydrated active hold replay err=%v", err)
	}
	if replayedHold.ID != hold.ID || replayedHold.BookingID != hold.BookingID {
		t.Fatalf("hydrated hold changed identity: got=%#v want=%#v", replayedHold, hold)
	}
	type checkoutResult struct {
		instruction *demand.CheckoutInstruction
		err         error
	}
	startCheckout := make(chan struct{})
	checkoutResults := make(chan checkoutResult, 2)
	var checkoutWG sync.WaitGroup
	for _, service := range []*demand.Service{second, peer} {
		service := service
		checkoutWG.Add(1)
		go func() {
			defer checkoutWG.Done()
			<-startCheckout
			instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
			checkoutResults <- checkoutResult{instruction: instruction, err: err}
		}()
	}
	close(startCheckout)
	checkoutWG.Wait()
	close(checkoutResults)

	var instruction *demand.CheckoutInstruction
	for result := range checkoutResults {
		if result.err != nil {
			t.Fatalf("concurrent checkout failed: %v", result.err)
		}
		if result.instruction == nil ||
			result.instruction.BookingID != hold.BookingID ||
			result.instruction.BookingState != "PENDING_PAYMENT" {
			t.Fatalf("checkout=%#v", result.instruction)
		}
		if instruction == nil {
			instruction = result.instruction
		} else if instruction.ID != result.instruction.ID || instruction.OrderID != result.instruction.OrderID {
			t.Fatalf("concurrent checkout changed durable identity: first=%#v second=%#v", instruction, result.instruction)
		}
	}

	third, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow.Add(time.Minute) }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	replayedInstruction, err := third.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatalf("checkout replay after restart failed: %v", err)
	}
	if replayedInstruction.ID != instruction.ID || replayedInstruction.OrderID != instruction.OrderID {
		t.Fatalf("checkout replay changed durable identity: got=%#v want=%#v", replayedInstruction, instruction)
	}

	var productID, directionID, productOwnerRef, firstAuthorRef, commercialOwnerRef, payoutBeneficiaryRef string
	var authorCount int
	if err := store.db.QueryRow(
		`SELECT product_id::text, organization_direction_id::text,
		        product_owner_ref, cardinality(author_refs), author_refs[1],
		        commercial_owner_ref, payout_beneficiary_ref
		   FROM orders
		  WHERE id = $1::uuid`,
		instruction.OrderID,
	).Scan(
		&productID,
		&directionID,
		&productOwnerRef,
		&authorCount,
		&firstAuthorRef,
		&commercialOwnerRef,
		&payoutBeneficiaryRef,
	); err != nil {
		t.Fatal(err)
	}
	expectedOrganizationID, err := persistentid.FromRef("catalog-specialist-organization", "spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	expectedDirectionID, err := persistentid.FromRef("catalog-specialist-direction", "spec-lebedeva:consultation")
	if err != nil {
		t.Fatal(err)
	}
	expectedProductID, err := persistentid.FromRef("catalog-specialist-product", "spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	expectedOwnerRef := "organization/" + expectedOrganizationID
	if productID != expectedProductID ||
		directionID != expectedDirectionID ||
		productOwnerRef != expectedOwnerRef ||
		authorCount != 1 ||
		firstAuthorRef != "identity-spec-lebedeva" ||
		commercialOwnerRef != expectedOwnerRef ||
		payoutBeneficiaryRef != "identity-spec-lebedeva" {
		t.Fatalf(
			"durable product snapshot product=%q direction=%q owner=%q authors=%d/%q commercial=%q beneficiary=%q",
			productID,
			directionID,
			productOwnerRef,
			authorCount,
			firstAuthorRef,
			commercialOwnerRef,
			payoutBeneficiaryRef,
		)
	}

	var (
		transactionRef          string
		sellerRef               string
		legalCommercialOwnerRef string
		paymentRecipientRef     string
		platformRole            string
		fiscalResponsibilityRef string
		refundResponsibilityRef string
		legalBeneficiaryRef     string
		legalPolicyVersion      string
	)
	if err := store.db.QueryRow(
		`SELECT
			transaction_ref,
			seller_or_service_provider_id,
			commercial_owner_id,
			payment_recipient_id,
			platform_role,
			fiscal_responsibility_id,
			refund_responsibility_id,
			payout_beneficiary_id,
			policy_version
		   FROM legal_transaction_snapshots
		  WHERE transaction_ref = $1`,
		"order/"+instruction.OrderID,
	).Scan(
		&transactionRef,
		&sellerRef,
		&legalCommercialOwnerRef,
		&paymentRecipientRef,
		&platformRole,
		&fiscalResponsibilityRef,
		&refundResponsibilityRef,
		&legalBeneficiaryRef,
		&legalPolicyVersion,
	); err != nil {
		t.Fatal(err)
	}
	if transactionRef != "order/"+instruction.OrderID ||
		sellerRef != "identity-spec-lebedeva" ||
		legalCommercialOwnerRef != expectedOwnerRef ||
		paymentRecipientRef != "identity-spec-lebedeva" ||
		platformRole != "MARKETPLACE_INTERMEDIARY" ||
		fiscalResponsibilityRef != "identity-spec-lebedeva" ||
		refundResponsibilityRef != "identity-spec-lebedeva" ||
		legalBeneficiaryRef != "identity-spec-lebedeva" ||
		legalPolicyVersion != "legal-conformance-v1" {
		t.Fatalf(
			"durable legal snapshot transaction=%q seller=%q commercial=%q recipient=%q platform=%q fiscal=%q refund=%q beneficiary=%q policy=%q",
			transactionRef,
			sellerRef,
			legalCommercialOwnerRef,
			paymentRecipientRef,
			platformRole,
			fiscalResponsibilityRef,
			refundResponsibilityRef,
			legalBeneficiaryRef,
			legalPolicyVersion,
		)
	}
	other, err := third.CreateIntent("тоже нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := third.ConfirmIntent(other.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := third.AcquireHold(other.ID, slots[0].ID, other.ClientIdentityID); !errors.Is(err, demand.ErrSlotBooked) {
		t.Fatalf("restart lost live booking exclusivity: err=%v", err)
	}

	event := demand.ProviderEvent{
		ProviderID:      instruction.ProviderID,
		ProviderEventID: "restart-proof-" + instruction.OrderID,
		OrderID:         instruction.OrderID,
		AmountMinor:     instruction.AmountMinor,
		Currency:        instruction.Currency,
		Outcome:         "CAPTURED",
	}
	type captureResult struct {
		evidence *demand.PaymentEvidence
		err      error
	}
	startCapture := make(chan struct{})
	results := make(chan captureResult, 2)
	var captureWG sync.WaitGroup
	for _, service := range []*demand.Service{second, third} {
		service := service
		captureWG.Add(1)
		go func() {
			defer captureWG.Done()
			<-startCapture
			evidence, err := service.ApplyProviderEvent(event)
			results <- captureResult{evidence: evidence, err: err}
		}()
	}
	close(startCapture)
	captureWG.Wait()
	close(results)

	var captured *demand.PaymentEvidence
	idempotentCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent provider capture failed: %v", result.err)
		}
		if result.evidence == nil || result.evidence.BookingState != "CONFIRMED" {
			t.Fatalf("concurrent capture=%#v", result.evidence)
		}
		if result.evidence.Idempotent {
			idempotentCount++
		}
		if captured == nil {
			captured = result.evidence
		} else if captured.LedgerEntryID != result.evidence.LedgerEntryID {
			t.Fatalf("concurrent capture created multiple ledger entries: first=%#v second=%#v", captured, result.evidence)
		}
	}
	if idempotentCount != 1 {
		t.Fatalf("concurrent capture idempotent responses=%d, want 1", idempotentCount)
	}
	entries, err := store.LedgerEntries()
	if err != nil {
		t.Fatal(err)
	}
	effectCount := 0
	for _, entry := range entries {
		if entry.EconomicEventRef == instruction.OrderID {
			effectCount++
		}
	}
	if effectCount != 1 {
		t.Fatalf("capture economic effect count=%d", effectCount)
	}

	if _, err := store.db.Exec(
		`UPDATE organization_directions
		    SET status = 'ARCHIVED', archived_at = now()
		  WHERE id = $1::uuid`,
		expectedDirectionID,
	); err != nil {
		t.Fatalf("archive sold direction: %v", err)
	}
	if _, err := store.db.Exec(
		`DELETE FROM organization_directions WHERE id = $1::uuid`,
		expectedDirectionID,
	); err == nil {
		t.Fatal("sold direction hard delete unexpectedly succeeded")
	}
	var archivedStatus, linkedProductID, linkedOrderID, linkedBookingID, linkedLedgerID string
	if err := store.db.QueryRow(
		`SELECT direction.status,
		        product.id::text,
		        order_row.id::text,
		        booking.id::text,
		        ledger.id::text
		   FROM organization_directions direction
		   JOIN products product
		     ON product.organization_direction_id = direction.id
		   JOIN orders order_row
		     ON order_row.product_id = product.id
		    AND order_row.organization_direction_id = direction.id
		   JOIN bookings booking
		     ON booking.id = order_row.booking_id
		   JOIN ledger_entries ledger
		     ON ledger.economic_event_ref = order_row.id::text
		  WHERE direction.id = $1::uuid`,
		expectedDirectionID,
	).Scan(
		&archivedStatus,
		&linkedProductID,
		&linkedOrderID,
		&linkedBookingID,
		&linkedLedgerID,
	); err != nil {
		t.Fatalf("read archived direction history chain: %v", err)
	}
	if archivedStatus != "ARCHIVED" ||
		linkedProductID != expectedProductID ||
		linkedOrderID != instruction.OrderID ||
		linkedBookingID != instruction.BookingID ||
		linkedLedgerID != captured.LedgerEntryID {
		t.Fatalf(
			"archived direction history drift status=%q product=%q order=%q booking=%q ledger=%q",
			archivedStatus,
			linkedProductID,
			linkedOrderID,
			linkedBookingID,
			linkedLedgerID,
		)
	}

	outboxKey := "booking-ledger:" + instruction.OrderID
	pendingOutbox, err := store.PendingOutbox(10)
	if err != nil {
		t.Fatal(err)
	}
	var durableEvent *eventspine.OutboxRecord
	for i := range pendingOutbox {
		if pendingOutbox[i].Event.IdempotencyKey == outboxKey {
			durableEvent = &pendingOutbox[i]
			break
		}
	}
	if durableEvent == nil ||
		durableEvent.Status != eventspine.Pending ||
		durableEvent.Event.EventType != "booking.ledger_committed" ||
		durableEvent.Event.AggregateRef != "booking/"+instruction.BookingID {
		t.Fatalf("durable booking outbox event missing or invalid: %#v", durableEvent)
	}

	deliveryCalls := 0
	effects := map[string]bool{}
	deliver := func(_ context.Context, event eventspine.EventEnvelope) error {
		deliveryCalls++
		effects[event.IdempotencyKey] = true
		return nil
	}
	if err := deliver(context.Background(), durableEvent.Event); err != nil {
		t.Fatal(err)
	}

	restartedOutbox, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedOutbox.Close()
	deliveredCount, err := restartedOutbox.DeliverPendingOutbox(context.Background(), 10, deliver)
	if err != nil {
		t.Fatalf("deliver pending outbox after restart: %v", err)
	}
	if deliveredCount != 1 || deliveryCalls != 2 || len(effects) != 1 {
		t.Fatalf(
			"restart delivery delivered=%d calls=%d distinct_effects=%d",
			deliveredCount,
			deliveryCalls,
			len(effects),
		)
	}
	pendingAfterDelivery, err := restartedOutbox.PendingOutbox(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range pendingAfterDelivery {
		if record.Event.IdempotencyKey == outboxKey {
			t.Fatalf("delivered event remained pending: %#v", record)
		}
	}
	var deliveryStatus string
	var deliveryAttempts int
	if err := restartedOutbox.db.QueryRow(
		`SELECT delivery_status, attempts
		   FROM outbox_events
		  WHERE idempotency_key = $1`,
		outboxKey,
	).Scan(&deliveryStatus, &deliveryAttempts); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "DELIVERED" || deliveryAttempts != 1 {
		t.Fatalf("durable outbox terminal state=%s attempts=%d", deliveryStatus, deliveryAttempts)
	}

	fourth, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow.Add(2 * time.Minute) }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	replayedCapture, err := fourth.ApplyProviderEvent(event)
	if err != nil {
		t.Fatalf("provider replay after restart failed: %v", err)
	}
	if !replayedCapture.Idempotent || replayedCapture.LedgerEntryID != captured.LedgerEntryID || replayedCapture.ID != captured.ID {
		t.Fatalf("provider replay changed durable effect: got=%#v want=%#v", replayedCapture, captured)
	}
	var outboxCount int
	if err := store.db.QueryRow(
		`SELECT count(*)
		   FROM outbox_events
		  WHERE idempotency_key = $1`,
		outboxKey,
	).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("provider replay created %d durable outbox events", outboxCount)
	}

	cancelled, err := fourth.CancelOrder(instruction.OrderID, "CLIENT_CANCEL")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.BookingState != "CANCELLED" || cancelled.Idempotent {
		t.Fatalf("cancel=%#v", cancelled)
	}

	fifth, err := demand.NewConformanceServiceWithStores(func() time.Time { return secondNow.Add(3 * time.Minute) }, store, store)
	if err != nil {
		t.Fatal(err)
	}
	replayedCancel, err := fifth.CancelOrder(instruction.OrderID, "CLIENT_CANCEL")
	if err != nil {
		t.Fatalf("cancel replay after restart failed: %v", err)
	}
	if !replayedCancel.Idempotent ||
		replayedCancel.ReversalLedgerID != cancelled.ReversalLedgerID ||
		replayedCancel.OriginalLedgerID != cancelled.OriginalLedgerID ||
		replayedCancel.ID != cancelled.ID {
		t.Fatalf("cancel replay changed durable reversal: got=%#v want=%#v", replayedCancel, cancelled)
	}

	futureNow := now.AddDate(0, 0, 10)
	future, err := demand.NewConformanceServiceWithStores(func() time.Time { return futureNow }, store, store)
	if err != nil {
		t.Fatalf("historical journey hydration failed after slot horizon rolled: %v", err)
	}
	futureMatches, _, err := future.Matches(intent.ID, "sleep")
	if err != nil || len(futureMatches) == 0 {
		t.Fatalf("historical intent unavailable after horizon rollover: matches=%#v err=%v", futureMatches, err)
	}
}
