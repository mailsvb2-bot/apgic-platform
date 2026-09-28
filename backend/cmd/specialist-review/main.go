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
		subjectType = flag.String("type", "", "review subject: evidence or profile")
		subjectID   = flag.String("id", "", "evidence UUID or specialist Identity UUID")
		decision    = flag.String("decision", "", "approve or reject")
		reviewer    = flag.String("reviewer", "", "stable operator/reviewer reference")
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
	if !ok {
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
	default:
		log.Fatal("-type must be evidence or profile")
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
