// finance-mfa-admin provisions a second factor only for an approved finance
// operator from a restricted server-side administrative session.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func run(args []string, databaseURL, keyEncoded string) (string, error) {
	flags := flag.NewFlagSet("finance-mfa-admin", flag.ContinueOnError)
	identity := flags.String("identity-id", "", "Existing approved finance operator Identity UUID")
	approver := flags.String("approved-by", "", "Human enrollment approver")
	reference := flags.String("approval-ref", "", "External signed approval reference")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*identity) == "" || strings.TrimSpace(*approver) == "" || strings.TrimSpace(*reference) == "" {
		return "", errors.New("requires --identity-id, --approved-by and --approval-ref")
	}
	keyBytes, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(keyEncoded))
	if err != nil || len(keyBytes) != 32 {
		return "", errors.New("APGIC_MFA_ENCRYPTION_KEY_BASE64 must decode to exactly 32 bytes")
	}
	var key [32]byte
	copy(key[:], keyBytes)
	if strings.TrimSpace(databaseURL) == "" {
		return "", errors.New("APGIC_DATABASE_URL is required")
	}
	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		return "", err
	}
	defer store.Close()
	return store.ProvisionFinanceMFA(context.Background(), *identity, *approver, *reference, key, time.Now().UTC())
}

func main() {
	secret, err := run(os.Args[1:], os.Getenv("APGIC_DATABASE_URL"), os.Getenv("APGIC_MFA_ENCRYPTION_KEY_BASE64"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "finance-mfa-admin:", err)
		os.Exit(2)
	}
	// Print ONCE on a private terminal; never log it as CI/staging proof.
	fmt.Fprintln(os.Stdout, "TOTP_SECRET_BASE32="+secret)
	fmt.Fprintln(os.Stderr, "Store the factor in the approved authenticator immediately; never paste into tickets, logs or GitHub.")
}
