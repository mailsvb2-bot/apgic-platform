package main

import "testing"

func TestParseDecision(t *testing.T) {
	tests := []struct {
		input    string
		approved bool
		ok       bool
	}{
		{input: "approve", approved: true, ok: true},
		{input: " APPROVE ", approved: true, ok: true},
		{input: "reject", approved: false, ok: true},
		{input: "deny", approved: false, ok: false},
		{input: "", approved: false, ok: false},
	}
	for _, test := range tests {
		approved, ok := parseDecision(test.input)
		if approved != test.approved || ok != test.ok {
			t.Fatalf("parseDecision(%q)=(%v,%v) want (%v,%v)",
				test.input, approved, ok, test.approved, test.ok)
		}
	}
}
