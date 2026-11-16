package runtimepostgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestAcademyVideoRangesCoverDurationRejectsMalformedRowsPostgres protects
// the database-side invariant independently of the Go attestation verifier.
// SQL aggregates ignore NULL inputs; a malformed extra range must never be
// hidden beside an otherwise valid full-duration range.
func TestAcademyVideoRangesCoverDurationRejectsMalformedRowsPostgres(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cases := []struct {
		name     string
		ranges   any
		duration int64
		want     bool
	}{
		{
			name: "single complete range",
			ranges: `[{"start_millis":0,"end_millis":60000}]`,
			duration: 60000, want: true,
		},
		{
			name: "contiguous complete ranges",
			ranges: `[{"start_millis":0,"end_millis":30000},{"start_millis":30000,"end_millis":60000}]`,
			duration: 60000, want: true,
		},
		{
			name: "gap",
			ranges: `[{"start_millis":0,"end_millis":20000},{"start_millis":30000,"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "overlap",
			ranges: `[{"start_millis":0,"end_millis":40000},{"start_millis":30000,"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "malformed missing start beside valid full range",
			ranges: `[{"start_millis":0,"end_millis":60000},{"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "malformed missing end beside valid full range",
			ranges: `[{"start_millis":0,"end_millis":60000},{"start_millis":0}]`,
			duration: 60000, want: false,
		},
		{
			name: "null endpoint",
			ranges: `[{"start_millis":0,"end_millis":60000},{"start_millis":null,"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "string endpoint",
			ranges: `[{"start_millis":0,"end_millis":60000},{"start_millis":"0","end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "fractional endpoint",
			ranges: `[{"start_millis":0,"end_millis":60000},{"start_millis":0.5,"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "negative endpoint",
			ranges: `[{"start_millis":-1,"end_millis":60000}]`,
			duration: 60000, want: false,
		},
		{
			name: "unexpected property",
			ranges: `[{"start_millis":0,"end_millis":60000,"provider_claim":"extra"}]`,
			duration: 60000, want: false,
		},
		{
			name: "non-object range",
			ranges: `[{"start_millis":0,"end_millis":60000},null]`,
			duration: 60000, want: false,
		},
		{
			name: "empty ranges",
			ranges: `[]`,
			duration: 60000, want: false,
		},
		{
			name: "non-array payload",
			ranges: `{"start_millis":0,"end_millis":60000}`,
			duration: 60000, want: false,
		},
		{
			name: "null payload",
			ranges: `null`,
			duration: 60000, want: false,
		},
		{
			name: "SQL null payload",
			ranges: nil,
			duration: 60000, want: false,
		},
		{
			name: "zero duration",
			ranges: `[{"start_millis":0,"end_millis":1}]`,
			duration: 0, want: false,
		},
		{
			name: "duration above provider maximum",
			ranges: `[{"start_millis":0,"end_millis":86400001}]`,
			duration: 86400001, want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var accepted bool
			err := db.QueryRowContext(context.Background(),
				"SELECT academy_video_ranges_cover_duration($1::jsonb,$2::bigint)",
				tc.ranges, tc.duration,
			).Scan(&accepted)
			if err != nil {
				t.Fatalf("range validation query failed: %v", err)
			}
			if accepted != tc.want {
				t.Fatalf("accepted=%v, want %v for ranges=%v duration=%d",
					accepted, tc.want, tc.ranges, tc.duration)
			}
		})
	}
}
