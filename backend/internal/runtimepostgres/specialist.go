package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

const specialistWriteTimeout = 3 * time.Second
const specialistQualificationPolicyVersion = "qualification-v1"

func (c *Checker) UpsertProfile(identityID, displayName, professionCode string) (specialist.Profile, error) {
	if c == nil || c.db == nil {
		return specialist.Profile{}, errors.New("postgres checker is not initialized")
	}
	if _, err := identity.New(identityID); err != nil {
		return specialist.Profile{}, err
	}
	displayName, professionCode, err := specialist.NormalizeProfile(displayName, professionCode)
	if err != nil {
		return specialist.Profile{}, err
	}
	profileID, err := persistentid.New()
	if err != nil {
		return specialist.Profile{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("begin specialist profile upsert: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING`,
		identityID,
	); err != nil {
		return specialist.Profile{}, fmt.Errorf("ensure specialist identity: %w", err)
	}
	var version int64
	if err := tx.QueryRowContext(ctx,
		`SELECT version FROM identities WHERE id = $1::uuid FOR UPDATE`,
		identityID,
	).Scan(&version); err != nil {
		return specialist.Profile{}, fmt.Errorf("lock specialist identity: %w", err)
	}
	roleResult, err := tx.ExecContext(ctx,
		`INSERT INTO identity_roles (identity_id, role_code)
		 VALUES ($1::uuid, 'SPECIALIST')
		 ON CONFLICT (identity_id, role_code) DO NOTHING`,
		identityID,
	)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("grant specialist role: %w", err)
	}
	if affected, _ := roleResult.RowsAffected(); affected == 1 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE identities SET version = version + 1 WHERE id = $1::uuid`,
			identityID,
		); err != nil {
			return specialist.Profile{}, fmt.Errorf("advance identity version for specialist role: %w", err)
		}
	}

	var existingID string
	var oldName, oldProfession string
	err = tx.QueryRowContext(ctx,
		`SELECT id::text, display_name, profession_code
		   FROM specialist_profiles
		  WHERE identity_id = $1::uuid
		  FOR UPDATE`,
		identityID,
	).Scan(&existingID, &oldName, &oldProfession)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO specialist_profiles (
				id, identity_id, display_name, profession_code, profile_complete, review_state
			 ) VALUES ($1::uuid, $2::uuid, $3, $4, true, 'PENDING')`,
			profileID, identityID, displayName, professionCode,
		); err != nil {
			return specialist.Profile{}, fmt.Errorf("create specialist profile: %w", err)
		}
	case err != nil:
		return specialist.Profile{}, fmt.Errorf("read specialist profile for upsert: %w", err)
	default:
		materialChange := oldName != displayName || oldProfession != professionCode
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_profiles
			    SET display_name = $2,
			        profession_code = $3,
			        profile_complete = true,
			        review_state = CASE WHEN $4 THEN 'PENDING' ELSE review_state END,
			        updated_at = now()
			  WHERE id = $1::uuid`,
			existingID, displayName, professionCode, materialChange,
		); err != nil {
			return specialist.Profile{}, fmt.Errorf("update specialist profile: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return specialist.Profile{}, fmt.Errorf("commit specialist profile upsert: %w", err)
	}
	return c.Profile(identityID)
}

func (c *Checker) Profile(identityID string) (specialist.Profile, error) {
	if c == nil || c.db == nil {
		return specialist.Profile{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("begin specialist profile snapshot: %w", err)
	}
	defer tx.Rollback()

	profile, err := loadSpecialistProfileTx(ctx, tx, identityID)
	if err != nil {
		return specialist.Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return specialist.Profile{}, fmt.Errorf("commit specialist profile snapshot: %w", err)
	}
	return profile, nil
}

func (c *Checker) DeclareCapability(identityID, topicID string) (specialist.Profile, error) {
	topicID, err := specialist.NormalizeTopic(topicID)
	if err != nil {
		return specialist.Profile{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("begin capability declaration: %w", err)
	}
	defer tx.Rollback()

	specialistID, err := specialistIDForIdentity(ctx, tx, identityID)
	if err != nil {
		return specialist.Profile{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO specialist_capabilities (
			specialist_id, topic_id, evidence_state, verification_state
		 ) VALUES ($1::uuid, $2, 'SELF_DECLARED', 'NOT_APPLICABLE')
		 ON CONFLICT (specialist_id, topic_id) DO NOTHING`,
		specialistID, topicID,
	); err != nil {
		return specialist.Profile{}, fmt.Errorf("declare specialist capability: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE specialist_profiles SET updated_at = now() WHERE id = $1::uuid`,
		specialistID,
	); err != nil {
		return specialist.Profile{}, fmt.Errorf("touch specialist profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return specialist.Profile{}, fmt.Errorf("commit capability declaration: %w", err)
	}
	return c.Profile(identityID)
}

func (c *Checker) SubmitEvidence(identityID, topicID, kind, reference string) (specialist.Profile, error) {
	topicID, err := specialist.NormalizeTopic(topicID)
	if err != nil {
		return specialist.Profile{}, err
	}
	kind, reference, err = specialist.NormalizeEvidence(kind, reference)
	if err != nil {
		return specialist.Profile{}, err
	}
	evidenceID, err := persistentid.New()
	if err != nil {
		return specialist.Profile{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("begin specialist evidence intake: %w", err)
	}
	defer tx.Rollback()

	specialistID, err := specialistIDForIdentity(ctx, tx, identityID)
	if err != nil {
		return specialist.Profile{}, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM specialist_capabilities
			WHERE specialist_id = $1::uuid AND topic_id = $2
		)`,
		specialistID, topicID,
	).Scan(&exists); err != nil {
		return specialist.Profile{}, fmt.Errorf("read capability for evidence intake: %w", err)
	}
	if !exists {
		return specialist.Profile{}, specialist.ErrCapabilityNotFound
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO specialist_evidence (
			id, specialist_id, topic_id, kind, evidence_ref
		 ) VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		 ON CONFLICT (specialist_id, topic_id, evidence_ref) DO NOTHING`,
		evidenceID, specialistID, topicID, kind, reference,
	)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("persist specialist evidence: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 1 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_capabilities
			    SET evidence_state = 'DOCUMENT_SUPPORTED',
			        verification_state = 'PENDING',
			        evidence_refs = CASE
			          WHEN $3 = ANY(evidence_refs) THEN evidence_refs
			          ELSE array_append(evidence_refs, $3)
			        END,
			        verified_at = NULL,
			        expires_at = NULL,
			        updated_at = now()
			  WHERE specialist_id = $1::uuid AND topic_id = $2`,
			specialistID, topicID, reference,
		); err != nil {
			return specialist.Profile{}, fmt.Errorf("advance capability to document-supported: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_profiles
			    SET review_state = 'MANUAL_REVIEW', updated_at = now()
			  WHERE id = $1::uuid`,
			specialistID,
		); err != nil {
			return specialist.Profile{}, fmt.Errorf("mark specialist profile for manual review: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return specialist.Profile{}, fmt.Errorf("commit specialist evidence intake: %w", err)
	}
	return c.Profile(identityID)
}

// ReviewEvidence is a trusted control-plane transition. It is intentionally not
// exposed by the public specialist HTTP surface.
func (c *Checker) ReviewEvidence(evidenceID string, approved bool, reviewerRef string) error {
	reviewerRef = strings.TrimSpace(reviewerRef)
	if reviewerRef == "" {
		return specialist.ErrEvidenceInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin specialist evidence review: %w", err)
	}
	defer tx.Rollback()

	var specialistID, topicID, state string
	if err := tx.QueryRowContext(ctx,
		`SELECT specialist_id::text, topic_id, state
		   FROM specialist_evidence
		  WHERE id = $1::uuid
		  FOR UPDATE`,
		evidenceID,
	).Scan(&specialistID, &topicID, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return specialist.ErrEvidenceInvalid
		}
		return fmt.Errorf("read specialist evidence for review: %w", err)
	}
	target := specialist.EvidenceRejected
	if approved {
		target = specialist.EvidenceAccepted
	}
	if state != specialist.EvidenceSubmitted {
		if state == target {
			return tx.Commit()
		}
		return specialist.ErrEvidenceInvalid
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE specialist_evidence
		    SET state = $2, reviewed_at = now(), reviewer_ref = $3
		  WHERE id = $1::uuid`,
		evidenceID, target, reviewerRef,
	); err != nil {
		return fmt.Errorf("review specialist evidence: %w", err)
	}
	var acceptedCount, submittedCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT
			count(*) FILTER (WHERE state = 'ACCEPTED'),
			count(*) FILTER (WHERE state = 'SUBMITTED')
		   FROM specialist_evidence
		  WHERE specialist_id = $1::uuid AND topic_id = $2`,
		specialistID, topicID,
	).Scan(&acceptedCount, &submittedCount); err != nil {
		return fmt.Errorf("recompute specialist evidence state: %w", err)
	}

	switch {
	case acceptedCount > 0:
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_capabilities
			    SET evidence_state = 'APGIC_VERIFIED',
			        verification_state = 'ACTIVE',
			        verified_at = COALESCE(verified_at, now()),
			        updated_at = now()
			  WHERE specialist_id = $1::uuid AND topic_id = $2`,
			specialistID, topicID,
		); err != nil {
			return fmt.Errorf("restore verified specialist capability: %w", err)
		}
	case submittedCount > 0:
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_capabilities
			    SET evidence_state = 'DOCUMENT_SUPPORTED',
			        verification_state = 'PENDING',
			        verified_at = NULL,
			        expires_at = NULL,
			        updated_at = now()
			  WHERE specialist_id = $1::uuid AND topic_id = $2`,
			specialistID, topicID,
		); err != nil {
			return fmt.Errorf("keep specialist capability pending review: %w", err)
		}
	default:
		if _, err := tx.ExecContext(ctx,
			`UPDATE specialist_capabilities
			    SET evidence_state = 'SELF_DECLARED',
			        verification_state = 'NOT_APPLICABLE',
			        verified_at = NULL,
			        expires_at = NULL,
			        updated_at = now()
			  WHERE specialist_id = $1::uuid AND topic_id = $2`,
			specialistID, topicID,
		); err != nil {
			return fmt.Errorf("return specialist capability to self-declared: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit specialist evidence review: %w", err)
	}
	return nil
}

// ReviewProfile is a trusted control-plane transition and records audit evidence.
// It is intentionally not exposed by the public specialist HTTP surface.
func (c *Checker) ReviewProfile(identityID string, approved bool, reviewerRef string) error {
	reviewerRef = strings.TrimSpace(reviewerRef)
	if reviewerRef == "" {
		return specialist.ErrProfileInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin specialist profile review: %w", err)
	}
	defer tx.Rollback()

	var specialistID, oldState string
	var complete bool
	if err := tx.QueryRowContext(ctx,
		`SELECT id::text, review_state, profile_complete
		   FROM specialist_profiles
		  WHERE identity_id = $1::uuid
		  FOR UPDATE`,
		identityID,
	).Scan(&specialistID, &oldState, &complete); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return specialist.ErrProfileNotFound
		}
		return fmt.Errorf("read specialist profile for review: %w", err)
	}
	target := "REJECTED"
	reason := "SPECIALIST_PROFILE_REJECTED"
	if approved {
		if !complete {
			return specialist.ErrProfileInvalid
		}
		target = "APPROVED"
		reason = "SPECIALIST_PROFILE_APPROVED"
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE specialist_profiles SET review_state = $2, updated_at = now() WHERE id = $1::uuid`,
		specialistID, target,
	); err != nil {
		return fmt.Errorf("review specialist profile: %w", err)
	}
	auditID, err := persistentid.New()
	if err != nil {
		return err
	}
	oldJSON, _ := json.Marshal(map[string]string{"review_state": oldState})
	newJSON, _ := json.Marshal(map[string]string{"review_state": target})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, old_state, new_state,
			reason, policy_version, correlation_id
		 ) VALUES ($1::uuid, $2, 'SPECIALIST_PROFILE_REVIEW', 'SPECIALIST',
			$3, $4::jsonb, $5::jsonb, $6, $7, $8)`,
		auditID, reviewerRef, "specialist:"+specialistID,
		string(oldJSON), string(newJSON), reason, specialistQualificationPolicyVersion,
		"specialist-review:"+specialistID,
	); err != nil {
		return fmt.Errorf("append specialist profile review audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit specialist profile review: %w", err)
	}
	return nil
}

func (c *Checker) Publish(identityID, topicID string) (specialist.PublishResult, error) {
	topicID, err := specialist.NormalizeTopic(topicID)
	if err != nil {
		return specialist.PublishResult{}, err
	}
	result := specialist.PublishResult{PolicyVersion: specialistQualificationPolicyVersion}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin specialist publish: %w", err)
	}
	defer tx.Rollback()

	var specialistID, profession, reviewState string
	var complete bool
	if err := tx.QueryRowContext(ctx,
		`SELECT id::text, profession_code, profile_complete, review_state
		   FROM specialist_profiles
		  WHERE identity_id = $1::uuid
		  FOR UPDATE`,
		identityID,
	).Scan(&specialistID, &profession, &complete, &reviewState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, specialist.ErrProfileNotFound
		}
		return result, fmt.Errorf("read specialist profile for publish: %w", err)
	}
	if !complete {
		result.ReasonCodes = []string{marketplace.ReasonProfileIncomplete}
		return result, tx.Commit()
	}
	if reviewState != string(marketplace.ReviewApproved) {
		result.ReasonCodes = []string{marketplace.ReasonReviewIncomplete}
		return result, tx.Commit()
	}

	var evidenceState, verificationState, evidenceJSON string
	if err := tx.QueryRowContext(ctx,
		`SELECT evidence_state, verification_state, to_json(evidence_refs)::text
		   FROM specialist_capabilities
		  WHERE specialist_id = $1::uuid AND topic_id = $2
		  FOR UPDATE`,
		specialistID, topicID,
	).Scan(&evidenceState, &verificationState, &evidenceJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			result.ReasonCodes = []string{marketplace.ReasonNoCapability}
			return result, tx.Commit()
		}
		return result, fmt.Errorf("read specialist capability for publish: %w", err)
	}

	var evidenceRefs []string
	if err := json.Unmarshal([]byte(evidenceJSON), &evidenceRefs); err != nil {
		return result, fmt.Errorf("decode specialist evidence refs: %w", err)
	}
	eligible := evidenceState == string(marketplace.EvidenceAPGICVerified) &&
		verificationState == specialist.VerificationActive &&
		specialistPolicyAllows(profession, topicID)

	evaluationID, err := persistentid.New()
	if err != nil {
		return result, err
	}
	decision := "INELIGIBLE"
	reasonCodes := []string{marketplace.ReasonNoEligibleTopic}
	if eligible {
		decision = "ELIGIBLE"
		reasonCodes = []string{marketplace.ReasonPublished}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO qualification_evaluations (
			id, specialist_id, topic_id, jurisdiction, service_format, age_group,
			requested_activity, decision, policy_version, reason_codes, evidence_refs,
			capability_evidence_state, capability_verification_state, evaluated_at
		 ) VALUES (
			$1::uuid, $2::uuid, $3, 'RU', 'ONLINE', 'ADULT',
			'DISCOVERY', $4, $5, $6, $7, $8, $9, now()
		 )`,
		evaluationID, specialistID, topicID, decision, specialistQualificationPolicyVersion,
		reasonCodes, evidenceRefs, evidenceState, verificationState,
	); err != nil {
		return result, fmt.Errorf("persist specialist qualification evaluation: %w", err)
	}
	if !eligible {
		result.ReasonCodes = reasonCodes
		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("commit blocked specialist publish: %w", err)
		}
		return result, nil
	}

	var active bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM specialist_publications
			WHERE specialist_id = $1::uuid AND topic_id = $2 AND state = 'ACTIVE'
		)`,
		specialistID, topicID,
	).Scan(&active); err != nil {
		return result, fmt.Errorf("read active specialist publication: %w", err)
	}
	if !active {
		publicationID, err := persistentid.New()
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO specialist_publications (
				id, specialist_id, topic_id, qualification_evaluation_id,
				state, reason_code, published_at
			 ) VALUES ($1::uuid, $2::uuid, $3, $4::uuid, 'ACTIVE', $5, now())`,
			publicationID, specialistID, topicID, evaluationID, marketplace.ReasonPublished,
		); err != nil {
			return result, fmt.Errorf("publish specialist capability: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit specialist publish: %w", err)
	}
	result.Allowed = true
	result.ReasonCodes = []string{marketplace.ReasonPublished}
	result.PublishedTopics = []string{topicID}
	return result, nil
}

func (c *Checker) Unpublish(identityID, topicID string) (specialist.PublishResult, error) {
	topicID, err := specialist.NormalizeTopic(topicID)
	if err != nil {
		return specialist.PublishResult{}, err
	}
	result := specialist.PublishResult{
		Allowed:       true,
		ReasonCodes:   []string{marketplace.ReasonUnpublished},
		PolicyVersion: specialistQualificationPolicyVersion,
	}
	ctx, cancel := context.WithTimeout(context.Background(), specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin specialist unpublish: %w", err)
	}
	defer tx.Rollback()
	specialistID, err := specialistIDForIdentity(ctx, tx, identityID)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE specialist_publications
		    SET state = 'UNPUBLISHED',
		        reason_code = $3,
		        ended_at = COALESCE(ended_at, now())
		  WHERE specialist_id = $1::uuid
		    AND topic_id = $2
		    AND state = 'ACTIVE'`,
		specialistID, topicID, marketplace.ReasonUnpublished,
	); err != nil {
		return result, fmt.Errorf("unpublish specialist capability: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit specialist unpublish: %w", err)
	}
	return result, nil
}

func specialistIDForIdentity(ctx context.Context, tx *sql.Tx, identityID string) (string, error) {
	var specialistID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id::text FROM specialist_profiles WHERE identity_id = $1::uuid`,
		identityID,
	).Scan(&specialistID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", specialist.ErrProfileNotFound
		}
		return "", fmt.Errorf("read specialist profile identity: %w", err)
	}
	return specialistID, nil
}

func loadSpecialistProfileTx(ctx context.Context, tx *sql.Tx, identityID string) (specialist.Profile, error) {
	var profile specialist.Profile
	if err := tx.QueryRowContext(ctx,
		`SELECT id::text, identity_id::text, display_name, profession_code, profile_complete, review_state
		   FROM specialist_profiles
		  WHERE identity_id = $1::uuid`,
		identityID,
	).Scan(
		&profile.ID,
		&profile.IdentityID,
		&profile.DisplayName,
		&profile.ProfessionCode,
		&profile.ProfileComplete,
		&profile.ReviewState,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return specialist.Profile{}, specialist.ErrProfileNotFound
		}
		return specialist.Profile{}, fmt.Errorf("read specialist profile: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT topic_id, evidence_state, verification_state, to_json(evidence_refs)::text
		   FROM specialist_capabilities
		  WHERE specialist_id = $1::uuid
		  ORDER BY topic_id`,
		profile.ID,
	)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("read specialist capabilities: %w", err)
	}
	for rows.Next() {
		var capability specialist.Capability
		var refsJSON string
		if err := rows.Scan(&capability.TopicID, &capability.EvidenceState, &capability.VerificationState, &refsJSON); err != nil {
			rows.Close()
			return specialist.Profile{}, fmt.Errorf("scan specialist capability: %w", err)
		}
		if err := json.Unmarshal([]byte(refsJSON), &capability.EvidenceRefs); err != nil {
			rows.Close()
			return specialist.Profile{}, fmt.Errorf("decode specialist capability evidence refs: %w", err)
		}
		profile.Capabilities = append(profile.Capabilities, capability)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return specialist.Profile{}, fmt.Errorf("iterate specialist capabilities: %w", err)
	}
	rows.Close()

	rows, err = tx.QueryContext(ctx,
		`SELECT id::text, topic_id, kind, evidence_ref, state, submitted_at, reviewed_at
		   FROM specialist_evidence
		  WHERE specialist_id = $1::uuid
		  ORDER BY submitted_at, id`,
		profile.ID,
	)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("read specialist evidence: %w", err)
	}
	for rows.Next() {
		var evidence specialist.Evidence
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&evidence.ID, &evidence.TopicID, &evidence.Kind, &evidence.Reference,
			&evidence.State, &evidence.SubmittedAt, &reviewedAt,
		); err != nil {
			rows.Close()
			return specialist.Profile{}, fmt.Errorf("scan specialist evidence: %w", err)
		}
		if reviewedAt.Valid {
			value := reviewedAt.Time
			evidence.ReviewedAt = &value
		}
		profile.Evidence = append(profile.Evidence, evidence)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return specialist.Profile{}, fmt.Errorf("iterate specialist evidence: %w", err)
	}
	rows.Close()

	rows, err = tx.QueryContext(ctx,
		`SELECT topic_id
		   FROM specialist_publications
		  WHERE specialist_id = $1::uuid AND state = 'ACTIVE'
		  ORDER BY topic_id`,
		profile.ID,
	)
	if err != nil {
		return specialist.Profile{}, fmt.Errorf("read specialist publications: %w", err)
	}
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			rows.Close()
			return specialist.Profile{}, fmt.Errorf("scan specialist publication: %w", err)
		}
		profile.PublishedTopics = append(profile.PublishedTopics, topic)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return specialist.Profile{}, fmt.Errorf("iterate specialist publications: %w", err)
	}
	rows.Close()

	if profile.Capabilities == nil {
		profile.Capabilities = []specialist.Capability{}
	}
	if profile.Evidence == nil {
		profile.Evidence = []specialist.Evidence{}
	}
	if profile.PublishedTopics == nil {
		profile.PublishedTopics = []string{}
	}
	return profile, nil
}

func specialistPolicyAllows(profession, topic string) bool {
	switch strings.ToUpper(strings.TrimSpace(profession)) {
	case "PSYCHOLOGIST":
		return topic == "anxiety" || topic == "sleep" || topic == "relationships"
	case "CAREER_COACH":
		return topic == "career"
	default:
		return false
	}
}
