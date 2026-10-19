package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

type operation struct {
	command, identityID, grantID, approvedBy, approvalRef, reason string
}

func parseArgs(args []string) (operation, error) {
	if len(args) == 0 {
		return operation{}, errors.New("usage: finance-operator-admin grant|revoke [flags]")
	}
	op := operation{command: args[0]}
	flags := flag.NewFlagSet("finance-operator-admin "+op.command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&op.identityID, "identity-id", "", "target APGIC Identity UUID (grant)")
	flags.StringVar(&op.grantID, "grant-id", "", "finance operator grant UUID (revoke)")
	flags.StringVar(&op.approvedBy, "approved-by", "", "control-plane approver or change reference")
	flags.StringVar(&op.approvalRef, "approval-ref", "", "ticket/change record authorizing the grant")
	flags.StringVar(&op.reason, "reason", "", "reason for revoking the grant")
	if err := flags.Parse(args[1:]); err != nil {
		return operation{}, err
	}
	switch op.command {
	case "grant":
		if op.identityID == "" || op.approvedBy == "" || op.approvalRef == "" || op.grantID != "" || op.reason != "" {
			return operation{}, errors.New("grant requires --identity-id, --approved-by and --approval-ref only")
		}
	case "revoke":
		if op.grantID == "" || op.approvedBy == "" || len(strings.TrimSpace(op.reason)) < 10 || op.identityID != "" || op.approvalRef != "" {
			return operation{}, errors.New("revoke requires --grant-id, --approved-by and --reason (at least 10 characters)")
		}
	default:
		return operation{}, errors.New("command must be grant or revoke")
	}
	return op, nil
}

func run(ctx context.Context, args []string, databaseURL string, out io.Writer) error {
	op, err := parseArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("APGIC_DATABASE_URL is required")
	}
	store, err := runtimepostgres.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("finance operator admin storage unavailable: %w", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	switch op.command {
	case "grant":
		id, err := store.GrantFinanceOperator(ctx, op.identityID, op.approvedBy, op.approvalRef, now)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "FINANCE_OPERATOR_GRANT_CREATED grant_id=%s identity_id=%s capability=FINANCE_DISCREPANCY_RESOLVE\n", id, op.identityID)
		return err
	case "revoke":
		if err := store.RevokeFinanceOperator(ctx, op.grantID, op.approvedBy, op.reason, now); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "FINANCE_OPERATOR_GRANT_REVOKED grant_id=%s\n", op.grantID)
		return err
	default:
		return errors.New("unsupported command")
	}
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv("APGIC_DATABASE_URL"), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "finance operator admin:", err)
		os.Exit(2)
	}
}
