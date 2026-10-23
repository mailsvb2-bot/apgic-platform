package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type event struct {
	EventID          string          `json:"event_id"`
	IdempotencyKey   string          `json:"idempotency_key"`
	EventType        string          `json:"event_type"`
	SchemaVersion    string          `json:"schema_version"`
	AggregateRef     string          `json:"aggregate_ref"`
	AggregateVersion uint64          `json:"aggregate_version"`
	OccurredAt       time.Time       `json:"occurred_at"`
	ProducedAt       time.Time       `json:"produced_at"`
	Producer         string          `json:"producer"`
	TenantScope      string          `json:"tenant_scope"`
	CorrelationID    string          `json:"correlation_id"`
	CausationID      string          `json:"causation_id"`
	Payload          json.RawMessage `json:"payload"`
}
type payload struct {
	BookingID        string `json:"booking_id"`
	BookingState     string `json:"booking_state"`
	LedgerEntryID    string `json:"ledger_entry_id"`
	EconomicEventRef string `json:"economic_event_ref"`
}
type handler struct {
	db               *sql.DB
	principal, token string
}

func constantEqual(a, b string) bool {
	ah := sha256.Sum256([]byte(a))
	bh := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ah[:], bh[:]) == 1 && len(a) == len(b)
}
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	if !constantEqual(r.Header.Get("X-APGIC-Service-Principal"), h.principal) ||
		!constantEqual(r.Header.Get("X-APGIC-Service-Scope"), "event:deliver") ||
		!constantEqual(r.Header.Get("Authorization"), "Bearer "+h.token) {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var e event
	if err := dec.Decode(&e); err != nil {
		http.Error(w, "invalid envelope", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid trailing data", 400)
		return
	}
	if e.EventType != "booking.ledger_committed" || e.SchemaVersion != "1" ||
		e.EventID == "" || e.IdempotencyKey == "" || e.AggregateVersion != 1 ||
		e.Producer != "booking-ledger" || len(e.Payload) == 0 {
		http.Error(w, "unsupported event", 422)
		return
	}
	var p payload
	pd := json.NewDecoder(strings.NewReader(string(e.Payload)))
	pd.DisallowUnknownFields()
	if pd.Decode(&p) != nil || p.BookingID == "" || p.LedgerEntryID == "" || p.EconomicEventRef == "" ||
		e.AggregateRef != "booking/"+p.BookingID || e.IdempotencyKey != "booking-ledger:"+p.EconomicEventRef {
		http.Error(w, "invalid booking event", 422)
		return
	}
	var extra any
	if pd.Decode(&extra) != io.EOF {
		http.Error(w, "invalid payload", 422)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := consume(ctx, h.db, e); err != nil {
		if errors.Is(err, errMismatch) {
			http.Error(w, "event conflict", 409)
			return
		}
		log.Printf("gateway transaction failed: %v", err)
		http.Error(w, "delivery not committed", 503)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("consumed\n"))
}

var errMismatch = errors.New("event does not match canonical source")

func consume(ctx context.Context, db *sql.DB, e event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Gateway and outbox use the same PostgreSQL database. Check persisted
	// canonical source and complete payload before acknowledging consumption.
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM outbox_events
 WHERE event_id=$1::uuid AND idempotency_key=$2 AND event_type=$3
   AND schema_version=$4 AND aggregate_ref=$5 AND aggregate_version=$6
