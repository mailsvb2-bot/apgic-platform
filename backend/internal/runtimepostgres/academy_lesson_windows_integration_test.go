package runtimepostgres

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Exercise the exact SQL expressions used by AcademyLearning, progress
// preflight and assessment submission against PostgreSQL's real clock.
// The database triggers in migration 000064 remain the final write authority.
func TestAcademyLessonWindowsAgreeWithCanonicalSQL(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated Academy PostgreSQL database not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	cases := []struct {
		name       string
		kind       string
		release    *time.Time
		deadline   *time.Time
		submission *time.Time
		lessonOpen bool
		available  bool
	}{
		{name: "unscheduled reading", kind: "READING", lessonOpen: true, available: true},
		{name: "unscheduled assessment", kind: "ASSESSMENT", lessonOpen: true, available: true},
		{name: "reading not released", kind: "READING", release: &future},
		{name: "assessment not released", kind: "ASSESSMENT", release: &future},
		{name: "reading expired", kind: "READING", deadline: &past},
		{name: "assessment expired", kind: "ASSESSMENT", deadline: &past},
		{name: "assessment submission closed", kind: "ASSESSMENT", submission: &past, lessonOpen: true},
		{name: "reading ignores assessment cutoff", kind: "READING", submission: &past, lessonOpen: true, available: true},
		{name: "assessment within both windows", kind: "ASSESSMENT", release: &past, deadline: &future, submission: &future, lessonOpen: true, available: true},
	}
	query := `SELECT (` + academyScheduleOpenSQL + `), (` + academyAssessmentOpenSQL + `)
 FROM (VALUES ($1::text)) AS l(kind)
 CROSS JOIN (VALUES ($2::timestamptz,$3::timestamptz)) AS sc(release_at,deadline_at)
 CROSS JOIN (VALUES ($4::timestamptz)) AS limits(submission_deadline)`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var lessonOpen, available bool
			if err := db.QueryRow(query, tc.kind, tc.release, tc.deadline, tc.submission).Scan(&lessonOpen, &available); err != nil {
				t.Fatal(err)
			}
			if lessonOpen != tc.lessonOpen || available != tc.available {
				t.Fatalf("schedule/read/write mismatch: lessonOpen=%t available=%t, expected %t/%t",
					lessonOpen, available, tc.lessonOpen, tc.available)
			}
		})
	}
}
