package marketcell

import (
	"testing"
	"math"
	"time"
)

func policy() Policy {
	return Policy{
		Version:      "market-cell-r5-ci-v1",
		MarketCellID: "ci-ru-anxiety-online",
		Thresholds: Thresholds{
			EligibleVerifiedSupplyMin:           3,
			ActiveSpecialistsMin:                3,
			BookableSlotCoverageMinPercent:      50,
			DutySupplyMin:                       0,
			TimeToAvailableSlotMedianMaxMinutes: 1440,
			TimeToAvailableSlotP95MaxMinutes:    4320,
			FillConversionMinPercent:            10,
			BookingConversionMinPercent:         5,
			CancellationMaxPercent:              30,
			NoShowMaxPercent:                    20,
			ResponseTimeP95MaxMinutes:           120,
			AcceptanceTimeP95MaxMinutes:         240,
			UnfilledDemandMaxPercent:            80,
			ComplaintRateMaxPercent:             20,
			SafetyIncidentRateMaxPercent:        5,
			ContributionMarginMinPercent:        -20,
		},
	}
}

func passingEvidence() EvidenceSnapshot {
	return EvidenceSnapshot{
		ID:           "market-cell-evidence-1",
		MarketCellID: "ci-ru-anxiety-online",
		ObservedAt:   time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
		EvidenceRefs: []string{
			"ci://market-cell/ru-anxiety-online/supply",
			"ci://market-cell/ru-anxiety-online/demand",
			"ci://market-cell/ru-anxiety-online/availability",
		},
		Metrics: Metrics{
			EligibleVerifiedSupply:           4,
			ActiveSpecialists:                4,
			BookableSlotCoveragePercent:      70,
			DutySupply:                       1,
			TimeToAvailableSlotMedianMinutes: 120,
			TimeToAvailableSlotP95Minutes:    480,
			FillConversionPercent:            35,
			BookingConversionPercent:         20,
			CancellationPercent:              10,
			NoShowPercent:                    5,
			ResponseTimeP95Minutes:           30,
			AcceptanceTimeP95Minutes:         45,
			UnfilledDemandPercent:            15,
			ComplaintRatePercent:             2,
			SafetyIncidentRatePercent:        0,
			ContributionMarginPercent:        5,
		},
	}
}

func TestScaleReadyRequiresRecordedEvidence(t *testing.T) {
	evidence := passingEvidence()
	evidence.EvidenceRefs = nil
	decision, err := EvaluateScaleReadiness(policy(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Eligible || decision.ReasonCode != ReasonEvidenceRequired {
		t.Fatalf("missing evidence must block scale: %#v", decision)
	}
}

func TestScaleReadyRequiresEveryThresholdToPass(t *testing.T) {
	evidence := passingEvidence()
	evidence.Metrics.EligibleVerifiedSupply = 2
	evidence.Metrics.TimeToAvailableSlotP95Minutes = 5000
	evidence.Metrics.UnfilledDemandPercent = 90

	decision, err := EvaluateScaleReadiness(policy(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Eligible || decision.ReasonCode != ReasonThresholdsNotMet {
		t.Fatalf("threshold breach must block scale: %#v", decision)
	}
	if len(decision.Breaches) != 3 {
		t.Fatalf("expected 3 explicit threshold breaches, got %#v", decision.Breaches)
	}
}

func TestScaleReadyOnlyWhenEvidenceAndThresholdsPass(t *testing.T) {
	decision, err := EvaluateScaleReadiness(policy(), passingEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Eligible || decision.ReasonCode != ReasonScaleReady {
		t.Fatalf("passing market cell should be scale-ready: %#v", decision)
	}
	if len(decision.Breaches) != 0 {
		t.Fatalf("passing decision cannot have breaches: %#v", decision.Breaches)
	}
}

func TestEvidenceForAnotherMarketCellCannotUnlockScale(t *testing.T) {
	evidence := passingEvidence()
	evidence.MarketCellID = "different-cell"
	if _, err := EvaluateScaleReadiness(policy(), evidence); err != ErrInvalidScaleInput {
		t.Fatalf("cross-cell evidence must fail closed, got %v", err)
	}
}

func TestScaleReadyStateTransitionRequiresEligibleDecision(t *testing.T) {
	cell := &MarketCell{ID: "ci-ru-anxiety-online", State: StateDemandTest}
	if err := cell.Transition(StateScaleReady, ScaleDecision{
		Eligible:      false,
		ReasonCode:    ReasonThresholdsNotMet,
		PolicyVersion: "market-cell-r5-ci-v1",
		EvidenceID:    "market-cell-evidence-1",
	}); err != ErrScaleTransitionDenied {
		t.Fatalf("ineligible decision must block SCALE_READY, got %v", err)
	}
	if cell.State != StateDemandTest {
		t.Fatalf("blocked scale transition mutated state to %s", cell.State)
	}

	decision, err := EvaluateScaleReadiness(policy(), passingEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Transition(StateScaleReady, decision); err != nil {
		t.Fatal(err)
	}
	if cell.State != StateScaleReady {
		t.Fatalf("state = %s", cell.State)
	}
}

func TestScaleReadyCannotBeSkippedFromDiscovery(t *testing.T) {
	cell := &MarketCell{ID: "ci-ru-anxiety-online", State: StateDiscovery}
	decision, err := EvaluateScaleReadiness(policy(), passingEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Transition(StateScaleReady, decision); err != ErrScaleTransitionDenied {
		t.Fatalf("discovery cannot jump directly to scale-ready, got %v", err)
	}
}

func TestScaleReadinessRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run("metric", func(t *testing.T) {
			ev := passingEvidence()
			ev.Metrics.BookingConversionPercent = value
			if decision, err := EvaluateScaleReadiness(policy(), ev); err != ErrInvalidScaleInput || decision.Eligible {
				t.Fatalf("non-finite conversion should fail closed: decision=%#v err=%v", decision, err)
			}
			ev = passingEvidence()
			ev.Metrics.TimeToAvailableSlotP95Minutes = value
			if decision, err := EvaluateScaleReadiness(policy(), ev); err != ErrInvalidScaleInput || decision.Eligible {
				t.Fatalf("non-finite duration should fail closed: decision=%#v err=%v", decision, err)
			}
			p := policy()
			p.Thresholds.ContributionMarginMinPercent = value
			if decision, err := EvaluateScaleReadiness(p, passingEvidence()); err != ErrInvalidScaleInput || decision.Eligible {
				t.Fatalf("non-finite threshold should fail closed: decision=%#v err=%v", decision, err)
			}
		})
	}
}

func TestScaleTransitionRejectsCrossCellAndForgedBreaches(t *testing.T) {
	decision, err := EvaluateScaleReadiness(policy(), passingEvidence())
	if err != nil { t.Fatal(err) }
	other := &MarketCell{ID:"different-cell",State:StateDemandTest}
	if err := other.Transition(StateScaleReady, decision); err != ErrScaleTransitionDenied {
		t.Fatalf("cross-cell decision was accepted: %v", err)
	}
	if other.State != StateDemandTest { t.Fatal("cross-cell attempt mutated state") }
	cell := &MarketCell{ID:policy().MarketCellID,State:StateDemandTest}
	decision.Breaches = []Breach{{Metric:"booking_conversion_percent"}}
	if err := cell.Transition(StateScaleReady, decision); err != ErrScaleTransitionDenied {
		t.Fatalf("decision with breaches was accepted: %v", err)
	}
}
