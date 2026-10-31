package academy

import (
	"errors"
	"testing"
	"time"
)

func TestAuthorizeCohortAdmission(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	base := CohortPolicy{Capacity: 2, Enrolled: 1, StartsAt: now.Add(2 * time.Hour), EnrollmentClosesAt: now.Add(time.Hour)}
	facts := CohortAdmissionFacts{Now: now, EntitlementActive: true, PrerequisitesComplete: true}
	if err := AuthorizeCohortAdmission(base, facts); err != nil {
		t.Fatalf("valid learner denied: %v", err)
	}
	cases := map[string]struct {
		policy CohortPolicy
		facts  CohortAdmissionFacts
	}{
		"full":          {CohortPolicy{Capacity: 1, Enrolled: 1, StartsAt: now.Add(time.Hour)}, facts},
		"no right":      {base, CohortAdmissionFacts{Now: now}},
		"duplicate":     {base, CohortAdmissionFacts{Now: now, EntitlementActive: true, AlreadyEnrolled: true}},
		"closed":        {CohortPolicy{Capacity: 2, StartsAt: now.Add(time.Hour), EnrollmentClosesAt: now}, facts},
		"prerequisite":  {CohortPolicy{Capacity: 2, StartsAt: now.Add(time.Hour), PrerequisitesRequired: true}, CohortAdmissionFacts{Now: now, EntitlementActive: true}},
		"invalid clock": {base, CohortAdmissionFacts{EntitlementActive: true}},
		"started":       {CohortPolicy{Capacity: 2, StartsAt: now}, facts},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := AuthorizeCohortAdmission(tc.policy, tc.facts); !errors.Is(err, ErrCohortAdmission) {
				t.Fatalf("expected cohort admission denial, got %v", err)
			}
		})
	}
}
