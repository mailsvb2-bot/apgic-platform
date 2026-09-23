package legal

import (
	"testing"
	"time"
)

func TestAcceptanceIsBoundToExactDocumentVersion(t *testing.T) {
	acceptance, err := NewAcceptance(
		"acceptance-1",
		"identity-1",
		"terms",
		"v1",
		"sha256:abc",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !acceptance.Covers("terms", "v1") {
		t.Fatal("v1 acceptance must cover v1")
	}
	if acceptance.Covers("terms", "v2") {
		t.Fatal("v1 acceptance must never imply acceptance of v2")
	}
}

func TestAcceptanceRequiresEvidence(t *testing.T) {
	_, err := NewAcceptance(
		"acceptance-1",
		"identity-1",
		"terms",
		"v1",
		"",
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("acceptance without evidence hash must fail")
	}
}
