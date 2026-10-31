package academy

import (
	"errors"
	"time"
)

var ErrCohortAdmission = errors.New("academy: cohort admission denied")

type CohortPolicy struct {
	Capacity              int
	Enrolled              int
	StartsAt              time.Time
	EnrollmentClosesAt    time.Time
	PrerequisitesRequired bool
}

type CohortAdmissionFacts struct {
	Now                   time.Time
	EntitlementActive     bool
	PrerequisitesComplete bool
	AlreadyEnrolled       bool
}

func AuthorizeCohortAdmission(c CohortPolicy, f CohortAdmissionFacts) error {
	if c.Capacity <= 0 || c.Enrolled < 0 || c.Enrolled >= c.Capacity || f.Now.IsZero() || !f.EntitlementActive || f.AlreadyEnrolled || c.StartsAt.IsZero() || !f.Now.Before(c.StartsAt) {
		return ErrCohortAdmission
	}
	if c.PrerequisitesRequired && !f.PrerequisitesComplete {
		return ErrCohortAdmission
	}
	if !c.EnrollmentClosesAt.IsZero() && !f.Now.Before(c.EnrollmentClosesAt) {
		return ErrCohortAdmission
	}
	return nil
}
