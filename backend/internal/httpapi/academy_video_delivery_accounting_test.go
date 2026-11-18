package httpapi

import (
	"io"
	"strings"
	"testing"
)

// A clean io.EOF from a short cloud body is not proof that the advertised
// representation reached the learner. Only flushed bytes may count.
func TestAcademyVideoDeliveredEndClampsShortReads(t *testing.T) {
	tests := []struct {
		name                        string
		start, end, delivered, want int64
	}{
		{"full", 0, 100, 100, 100},
		{"short-clean-eof", 0, 100, 12, 12},
		{"partial-range-short", 40, 100, 12, 52},
		{"exact-range", 40, 100, 60, 100},
		{"over-delivery", 40, 100, 100, 100},
		{"zero", 40, 100, 0, 40},
		{"negative", 40, 100, -1, 40},
		{"invalid-interval", 100, 40, 10, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := academyVideoDeliveredEnd(tc.start, tc.end, tc.delivered); got != tc.want {
				t.Fatalf("delivered interval end = %d, want %d", got, tc.want)
			}
		})
	}
}

// Exercise the actual io.Copy contract: a source can return a clean EOF
// before the advertised representation length. A nil copy error must not
// cause the canonical Academy delivery writer to credit missing bytes.
func TestAcademyVideoCleanEOFDoesNotCreditUndeliveredBytes(t *testing.T) {
	const advertisedLength int64 = 128
	var learner strings.Builder
	copied, err := io.Copy(&learner, strings.NewReader(strings.Repeat("x", 17)))
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied != 17 || int64(learner.Len()) != copied {
		t.Fatalf("copied=%d flushed=%d, want 17", copied, learner.Len())
	}
	if got := academyVideoDeliveredEnd(0, advertisedLength, int64(learner.Len())); got != 17 {
		t.Fatalf("credited %d bytes from a short clean EOF, want 17", got)
	}
}
