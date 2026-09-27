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
)

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
