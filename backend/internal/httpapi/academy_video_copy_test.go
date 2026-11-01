package httpapi

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAcademyCopyVideoAuthorizedStopsBeforeRevokedChunk(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	err := academyCopyVideoAuthorized(&output, strings.NewReader(strings.Repeat("v", 196608)), func() bool {
		calls++
		return calls == 1
	}, 65536, time.Hour)
	if !errors.Is(err, errAcademyVideoRevoked) {
		t.Fatalf("expected revoked error, got %v", err)
	}
	if output.Len() != 65536 {
		t.Fatalf("revoked bytes escaped: got %d, want 65536", output.Len())
	}
}

func TestAcademyCopyVideoAuthorizedAllowsFullContent(t *testing.T) {
	input := strings.Repeat("v", 131072)
	var output bytes.Buffer
	if err := academyCopyVideoAuthorized(&output, strings.NewReader(input), func() bool { return true }, 65536, time.Second); err != nil {
		t.Fatal(err)
	}
	if output.String() != input {
		t.Fatal("authorized output was truncated")
	}
}
