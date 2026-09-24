package marketcell

import (
	"errors"
	"strings"
	"time"
)

const (
	ReasonScaleReady          = "MARKET_CELL_SCALE_READY"
	ReasonEvidenceRequired    = "MARKET_CELL_EVIDENCE_REQUIRED"
	ReasonThresholdsNotMet    = "MARKET_CELL_THRESHOLDS_NOT_MET"
	ReasonInvalidScaleInput   = "MARKET_CELL_SCALE_INPUT_INVALID"
)

var ErrInvalidScaleInput = errors.New("invalid market cell scale input")

type Thresholds struct {
	EligibleVerifiedSupplyMin              int
	ActiveSpecialistsMin                   int
	BookableSlotCoverageMinPercent         float64
	DutySupplyMin                          int
	TimeToAvailableSlotMedianMaxMinutes    float64
	TimeToAvailableSlotP95MaxMinutes       float64
	FillConversionMinPercent               float64
	BookingConversionMinPercent            float64
	CancellationMaxPercent                 float64
	NoShowMaxPercent                       float64
	ResponseTimeP95MaxMinutes              float64
	AcceptanceTimeP95MaxMinutes            float64
	UnfilledDemandMaxPercent               float64
	ComplaintRateMaxPercent                float64
	SafetyIncidentRateMaxPercent           float64
	ContributionMarginMinPercent           float64
}

type Metrics struct {
	EligibleVerifiedSupply                 int
	ActiveSpecialists                      int
	BookableSlotCoveragePercent            float64
	DutySupply                              int
	TimeToAvailableSlotMedianMinutes        float64
	TimeToAvailableSlotP95Minutes           float64
	FillConversionPercent                  float64
	BookingConversionPercent               float64
	CancellationPercent                    float64
	NoShowPercent                          float64
	ResponseTimeP95Minutes                 float64
	AcceptanceTimeP95Minutes               float64
	UnfilledDemandPercent                  float64
	ComplaintRatePercent                   float64
	SafetyIncidentRatePercent              float64
	ContributionMarginPercent              float64
}

type Policy struct {
	Version      string
	MarketCellID string
	Thresholds   Thresholds
}

type EvidenceSnapshot struct {
	ID           string
	MarketCellID string
	ObservedAt   time.Time
	EvidenceRefs []string
	Metrics      Metrics
}

type Breach struct {
	Metric    string
	Observed  float64
	Threshold float64
	Operator  string
}

type ScaleDecision struct {
	Eligible      bool
	ReasonCode    string
	PolicyVersion string
	EvidenceID    string
	Breaches      []Breach
}

func EvaluateScaleReadiness(policy Policy, evidence EvidenceSnapshot) (ScaleDecision, error) {
	base := ScaleDecision{
		ReasonCode:    ReasonInvalidScaleInput,
		PolicyVersion: policy.Version,
		EvidenceID:    evidence.ID,
	}
	if err := validatePolicy(policy); err != nil {
		return base, err
	}
	if strings.TrimSpace(evidence.ID) == "" ||
		evidence.MarketCellID != policy.MarketCellID ||
		evidence.ObservedAt.IsZero() {
		return base, ErrInvalidScaleInput
	}
	if len(evidence.EvidenceRefs) == 0 {
		base.ReasonCode = ReasonEvidenceRequired
		return base, nil
	}
	for _, ref := range evidence.EvidenceRefs {
		if strings.TrimSpace(ref) == "" {
			return base, ErrInvalidScaleInput
		}
	}
	if err := validateMetrics(evidence.Metrics); err != nil {
		return base, err
	}

	t := policy.Thresholds
	m := evidence.Metrics
	breaches := make([]Breach, 0)

	min := func(name string, observed, threshold float64) {
		if observed < threshold {
			breaches = append(breaches, Breach{
				Metric: name, Observed: observed, Threshold: threshold, Operator: ">=",
			})
		}
	}
	max := func(name string, observed, threshold float64) {
		if observed > threshold {
			breaches = append(breaches, Breach{
				Metric: name, Observed: observed, Threshold: threshold, Operator: "<=",
			})
		}
	}

	min("eligible_verified_supply", float64(m.EligibleVerifiedSupply), float64(t.EligibleVerifiedSupplyMin))
	min("active_specialists", float64(m.ActiveSpecialists), float64(t.ActiveSpecialistsMin))
	min("bookable_slot_coverage_percent", m.BookableSlotCoveragePercent, t.BookableSlotCoverageMinPercent)
	min("duty_supply", float64(m.DutySupply), float64(t.DutySupplyMin))
	max("time_to_available_slot_median_minutes", m.TimeToAvailableSlotMedianMinutes, t.TimeToAvailableSlotMedianMaxMinutes)
	max("time_to_available_slot_p95_minutes", m.TimeToAvailableSlotP95Minutes, t.TimeToAvailableSlotP95MaxMinutes)
	min("fill_conversion_percent", m.FillConversionPercent, t.FillConversionMinPercent)
	min("booking_conversion_percent", m.BookingConversionPercent, t.BookingConversionMinPercent)
	max("cancellation_percent", m.CancellationPercent, t.CancellationMaxPercent)
	max("no_show_percent", m.NoShowPercent, t.NoShowMaxPercent)
	max("response_time_p95_minutes", m.ResponseTimeP95Minutes, t.ResponseTimeP95MaxMinutes)
	max("acceptance_time_p95_minutes", m.AcceptanceTimeP95Minutes, t.AcceptanceTimeP95MaxMinutes)
	max("unfilled_demand_percent", m.UnfilledDemandPercent, t.UnfilledDemandMaxPercent)
	max("complaint_rate_percent", m.ComplaintRatePercent, t.ComplaintRateMaxPercent)
	max("safety_incident_rate_percent", m.SafetyIncidentRatePercent, t.SafetyIncidentRateMaxPercent)
	min("contribution_margin_percent", m.ContributionMarginPercent, t.ContributionMarginMinPercent)

	if len(breaches) != 0 {
		base.ReasonCode = ReasonThresholdsNotMet
		base.Breaches = breaches
		return base, nil
	}

	base.Eligible = true
	base.ReasonCode = ReasonScaleReady
	return base, nil
}

func validatePolicy(policy Policy) error {
	if strings.TrimSpace(policy.Version) == "" || strings.TrimSpace(policy.MarketCellID) == "" {
		return ErrInvalidScaleInput
	}
	t := policy.Thresholds
	if t.EligibleVerifiedSupplyMin < 0 ||
		t.ActiveSpecialistsMin < 0 ||
		t.DutySupplyMin < 0 ||
		t.TimeToAvailableSlotMedianMaxMinutes < 0 ||
		t.TimeToAvailableSlotP95MaxMinutes < 0 ||
		t.ResponseTimeP95MaxMinutes < 0 ||
		t.AcceptanceTimeP95MaxMinutes < 0 {
		return ErrInvalidScaleInput
	}
	for _, value := range []float64{
		t.BookableSlotCoverageMinPercent,
		t.FillConversionMinPercent,
		t.BookingConversionMinPercent,
		t.CancellationMaxPercent,
		t.NoShowMaxPercent,
		t.UnfilledDemandMaxPercent,
		t.ComplaintRateMaxPercent,
		t.SafetyIncidentRateMaxPercent,
	} {
		if value < 0 || value > 100 {
			return ErrInvalidScaleInput
		}
	}
	if t.ContributionMarginMinPercent < -100 || t.ContributionMarginMinPercent > 100 {
		return ErrInvalidScaleInput
	}
	return nil
}

func validateMetrics(metrics Metrics) error {
	if metrics.EligibleVerifiedSupply < 0 ||
		metrics.ActiveSpecialists < 0 ||
		metrics.DutySupply < 0 ||
		metrics.TimeToAvailableSlotMedianMinutes < 0 ||
		metrics.TimeToAvailableSlotP95Minutes < 0 ||
		metrics.ResponseTimeP95Minutes < 0 ||
		metrics.AcceptanceTimeP95Minutes < 0 {
		return ErrInvalidScaleInput
	}
	for _, value := range []float64{
		metrics.BookableSlotCoveragePercent,
		metrics.FillConversionPercent,
		metrics.BookingConversionPercent,
		metrics.CancellationPercent,
		metrics.NoShowPercent,
		metrics.UnfilledDemandPercent,
		metrics.ComplaintRatePercent,
		metrics.SafetyIncidentRatePercent,
	} {
		if value < 0 || value > 100 {
			return ErrInvalidScaleInput
		}
	}
	if metrics.ContributionMarginPercent < -100 || metrics.ContributionMarginPercent > 100 {
		return ErrInvalidScaleInput
	}
	return nil
}
