package main

import "testing"

func validEvidence() evidence {
	return evidence{
		SchemaVersion:               "conn001-staging-core-continuity-v1",
		EvidenceType:                "STAGING_PROVIDER_NEUTRAL_CORE_PROOF",
		CandidateSHA:                "candidate",
		ObservedAt:                  "2026-10-08T00:00:00Z",
		Environment:                 "STAGING",
		ConnectorCapability:         "COMMUNICATION_PROVIDER",
		ConnectorStatus:             "DISABLED",
		IdentityPersisted:           true,
		BookingHoldPersisted:        true,
		BookingState:                "HELD",
		LedgerEffectCount:           1,
		ProviderExecutionClaimed:    false,
		CanonicalTruthProviderOwned: false,
		BusinessTruthSurvived:       true,
		ProductionEvidence:          false,
	}
}

func TestEvidenceAcceptsProviderNeutralStagingProof(t *testing.T) {
	if err := validEvidence().validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceRejectsFabricatedProviderExecution(t *testing.T) {
	proof := validEvidence()
	proof.ProviderExecutionClaimed = true
	if err := proof.validate(); err == nil {
		t.Fatal("expected fabricated provider execution to be rejected")
	}
}

func TestEvidenceRejectsProductionClaim(t *testing.T) {
	proof := validEvidence()
	proof.ProductionEvidence = true
	if err := proof.validate(); err == nil {
		t.Fatal("expected staging proof production claim to be rejected")
	}
}

func TestEvidenceRejectsMissingCanonicalTruth(t *testing.T) {
	proof := validEvidence()
	proof.LedgerEffectCount = 0
	proof.BusinessTruthSurvived = false
	if err := proof.validate(); err == nil {
		t.Fatal("expected incomplete canonical truth proof to be rejected")
	}
}
