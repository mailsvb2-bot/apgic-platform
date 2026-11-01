package runtimepostgres

import (
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyMyCertificates reads only immutable credentials issued to the signed
// learner. Refunds revoke lesson access, not historical certificate visibility.
// Revoked certificates remain visible with their actual canonical state.
func (c *Checker) AcademyMyCertificates(actor string) ([]academy.LearnerCertificate, error) {
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	if actor == "" {
		return nil, academy.ErrDenied
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT cert.id::text, e.id::text,
	 COALESCE(v.title_snapshot,c.title), v.version_no, cert.state,
	 cert.verification_token, candidate.state, cert.issued_at,
	 CASE WHEN academy_entitlement_is_active(e.entitlement_id)
	   THEN 'ACTIVE' ELSE 'REVOKED' END
	 FROM academy_certificates cert
	 JOIN academy_enrollments e ON e.id=cert.enrollment_id
	 JOIN academy_course_versions v ON v.id=e.version_id
	 JOIN academy_courses c ON c.id=v.course_id
	 JOIN academy_evidence_candidates candidate ON candidate.certificate_id=cert.id
	   AND candidate.candidate_identity_id=e.identity_id
	 WHERE e.identity_id=$1::uuid
	 ORDER BY cert.issued_at DESC,cert.id DESC LIMIT 100`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	certificates := []academy.LearnerCertificate{}
	for rows.Next() {
		var cert academy.LearnerCertificate
		var token string
		if err := rows.Scan(&cert.ID, &cert.EnrollmentID, &cert.CourseTitle,
			&cert.CourseVersion, &cert.State, &token, &cert.EvidenceCandidateState,
			&cert.IssuedAt, &cert.LearningAccessState); err != nil {
			return nil, err
		}
		// The opaque token is returned only as a same-origin verification URL,
		// never as raw content or an authorization bypass for learning.
		cert.VerificationURL = "/v1/academy/certificates/verify/" + token
		certificates = append(certificates, cert)
	}
	return certificates, rows.Err()
}
