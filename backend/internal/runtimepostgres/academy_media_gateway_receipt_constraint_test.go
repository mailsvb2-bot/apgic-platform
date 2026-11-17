package runtimepostgres

import (
 "database/sql"
 "fmt"
 "os"
 "strings"
 "testing"

 _ "github.com/jackc/pgx/v5/stdlib"
)

// Exercises the migrated *actual* receipt check in a temporary table,
// avoiding mutations to durable gateway/outbox facts.
func TestAcademyMediaAssignedGatewayReceiptConstraintPostgres(t *testing.T) {
 dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
 if dsn == "" { t.Skip("isolated migrated PostgreSQL required") }
 db,err:=sql.Open("pgx",dsn); if err!=nil{t.Fatal(err)};defer db.Close()
 tx,err:=db.Begin();if err!=nil{t.Fatal(err)};defer tx.Rollback()
 var definition string
 if err:=tx.QueryRow(`SELECT pg_get_constraintdef(oid) FROM pg_constraint
   WHERE conrelid='event_gateway_receipts'::regclass
   AND conname='event_gateway_receipts_event_type_check'`).Scan(&definition);err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(definition,"CHECK (") {t.Fatalf("not a check constraint: %q",definition)}
 if _,err:=tx.Exec("CREATE TEMP TABLE apgic_receipt_type_probe (event_type text NOT NULL) ON COMMIT DROP");err!=nil{t.Fatal(err)}
 if _,err:=tx.Exec(fmt.Sprintf("ALTER TABLE pg_temp.apgic_receipt_type_probe ADD CONSTRAINT receipt_type_probe %s",definition));err!=nil{t.Fatal(err)}
 for _,kind:=range []string{"academy.media.assigned","academy.lesson.added","booking.ledger_committed"}{
  if _,err:=tx.Exec("INSERT INTO pg_temp.apgic_receipt_type_probe(event_type) VALUES($1)",kind);err!=nil{t.Fatalf("%s unexpectedly rejected: %v",kind,err)}
 }
 if _,err:=tx.Exec("SAVEPOINT invalid_type");err!=nil{t.Fatal(err)}
 if _,err:=tx.Exec("INSERT INTO pg_temp.apgic_receipt_type_probe(event_type) VALUES('academy.unauthorized.event')");err==nil{t.Fatal("unknown gateway receipt event was admitted")}
 if _,err:=tx.Exec("ROLLBACK TO SAVEPOINT invalid_type");err!=nil{t.Fatal(err)}
}
