package academy

// CanSubmitAssessmentAttempt is a fail-closed policy shared with the
// transactional PostgreSQL Academy owner. A pending human review must be
// resolved before another attempt; passing an assessment is final.
func CanSubmitAssessmentAttempt(hasPendingOrPassed bool) error {
	if hasPendingOrPassed {
		return ErrState
	}
	return nil
}
