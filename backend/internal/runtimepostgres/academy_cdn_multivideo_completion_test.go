package runtimepostgres

import (
 "context"
 "database/sql"
 "os"
 "testing"

 _ "github.com/jackc/pgx/v5/stdlib"
)

// The canonical CDN completion predicate must be fail-closed for absent
// enrollment or lesson IDs, including its NULL arguments.
func TestAcademyCDNPlaybackCompleteFailsClosedWithoutAssignments(t *testing.T) {
 dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
 if dsn == "" { t.Skip("isolated migrated PostgreSQL required") }
 db, err := sql.Open("pgx", dsn)
 if err != nil { t.Fatal(err) }
 defer db.Close()
 for _, tc := range []struct{name string; enrollment any; lesson any}{
  {"missing enrollment and lesson", "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"},
  {"null enrollment", nil, "00000000-0000-0000-0000-000000000002"},
  {"null lesson", "00000000-0000-0000-0000-000000000001", nil},
 } {
  t.Run(tc.name, func(t *testing.T) {
   var complete bool
   if err := db.QueryRowContext(context.Background(),
    "SELECT academy_cdn_video_playback_complete($1::uuid,$2::uuid)",
    tc.enrollment,tc.lesson).Scan(&complete); err != nil {t.Fatal(err)}
   if complete {t.Fatal("missing assignment was accepted as complete CDN playback")}
  })
 }
}
