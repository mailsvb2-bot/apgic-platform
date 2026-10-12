package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func main() {
	var (
		subjectType = flag.String("type", "", "review subject: evidence, profile or capability")
		subjectID   = flag.String("id", "", "evidence UUID or specialist Identity UUID")
		decision    = flag.String("decision", "", "approve/reject or revoke for capability")
		reviewer    = flag.String("reviewer", "", "stable operator/reviewer reference")
		topic       = flag.String("topic", "", "topic required for capability revocation")
	)
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("APGIC_DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("APGIC_DATABASE_URL is required")
	}
	if strings.TrimSpace(*subjectID) == "" || strings.TrimSpace(*reviewer) == "" {
		log.Fatal("-id and -reviewer are required")
	}
	approved, ok := parseDecision(*decision)
	if strings.EqualFold(*subjectType, "capability") {
		if !strings.EqualFold(*decision, "revoke") || strings.TrimSpace(*topic) == "" {
			log.Fatal("capability requires -decision revoke and -topic")
		}
	} else if !ok {
		log.Fatal("-decision must be approve or reject")
	}

	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("open APGIC PostgreSQL: %v", err)
	}
	defer store.Close()

	switch strings.ToLower(strings.TrimSpace(*subjectType)) {
	case "evidence":
		err = store.ReviewEvidence(*subjectID, approved, *reviewer)
	case "profile":
		err = store.ReviewProfile(*subjectID, approved, *reviewer)
	case "capability":
		err = store.RevokeCapability(*subjectID, *topic, *reviewer)
	default:
		log.Fatal("-type must be evidence, profile or capability")
	}
	if err != nil {
		log.Fatalf("specialist review failed: %v", err)
	}
	fmt.Printf("specialist review applied: type=%s id=%s decision=%s reviewer=%s\n",
		strings.ToLower(strings.TrimSpace(*subjectType)),
		*subjectID,
		strings.ToLower(strings.TrimSpace(*decision)),
		*reviewer,
	)
}

func parseDecision(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve":
		return true, true
	case "reject":
		return false, true
	default:
		return false, false
	}
}
