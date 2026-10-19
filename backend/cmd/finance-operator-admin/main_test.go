package main

import "testing"

func TestParseGrantRequiresExplicitApprovalAndReference(t *testing.T) {
	if _, err := parseArgs([]string{"grant", "--identity-id", "00000000-0000-0000-0000-000000000001"}); err == nil {
		t.Fatal("grant without approval metadata was accepted")
	}
	op, err := parseArgs([]string{"grant", "--identity-id", "00000000-0000-0000-0000-000000000001", "--approved-by", "change-control", "--approval-ref", "CHG-2026-1015"})
	if err != nil || op.command != "grant" || op.approvalRef != "CHG-2026-1015" {
		t.Fatalf("valid grant request rejected: %#v %v", op, err)
	}
}

func TestParseRevokeRequiresReasonAndApproval(t *testing.T) {
	if _, err := parseArgs([]string{"revoke", "--grant-id", "00000000-0000-0000-0000-000000000001", "--approved-by", "change-control", "--reason", "short"}); err == nil {
		t.Fatal("short revocation reason accepted")
	}
	op, err := parseArgs([]string{"revoke", "--grant-id", "00000000-0000-0000-0000-000000000001", "--approved-by", "change-control", "--reason", "operator role no longer required"})
	if err != nil || op.command != "revoke" {
		t.Fatalf("valid revocation request rejected: %#v %v", op, err)
	}
}
