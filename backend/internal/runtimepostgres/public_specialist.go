package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// PublicProfiles and PublicProfile read ONLY the current canonical publication,
// reviewed profile, verified non-expired capability and recorded eligible policy.
// No client-provided verification flag, evidence references or private Identity ID
// can leak through these projections. Indexes are rebuildable, not authoritative.
const publicSpecialistQuery = `
WITH eligible AS (
 SELECT profile.id, profile.display_name, profile.profession_code, publication.topic_id
 FROM specialist_profiles profile
 JOIN specialist_publications publication ON publication.specialist_id=profile.id AND publication.state='ACTIVE'
 JOIN specialist_capabilities capability ON capability.specialist_id=profile.id
   AND capability.topic_id=publication.topic_id
   AND capability.evidence_state='APGIC_VERIFIED'
   AND capability.verification_state='ACTIVE'
   AND (capability.expires_at IS NULL OR capability.expires_at>now())
 JOIN qualification_evaluations decision ON decision.id=publication.qualification_evaluation_id
   AND decision.decision='ELIGIBLE' AND decision.policy_version='qualification-v1'
   AND decision.specialist_id=profile.id AND decision.topic_id=publication.topic_id
 WHERE profile.profile_complete AND profile.review_state='APPROVED'
   AND ((profile.profession_code='PSYCHOLOGIST' AND publication.topic_id IN ('anxiety','sleep','relationships'))
        OR (profile.profession_code='CAREER_COACH' AND publication.topic_id='career'))
)
SELECT eligible.id::text, eligible.display_name, eligible.profession_code,
 COALESCE(details.headline,''), COALESCE(details.about,''),
 COALESCE(details.experience_years,0),
 COALESCE(to_json(details.languages)::text,'["ru"]'),
 COALESCE(to_json(details.limitations)::text,'[]'),
 to_json(array_agg(DISTINCT eligible.topic_id ORDER BY eligible.topic_id))::text,
 COALESCE(ratings.review_count,0),
 CASE WHEN ratings.review_count >= 5 THEN ratings.rating_average ELSE NULL END
FROM eligible LEFT JOIN specialist_public_details details ON details.specialist_id=eligible.id
LEFT JOIN (
 SELECT specialist_id, count(*)::integer review_count, avg(rating)::float8 rating_average
 FROM specialist_verified_feedback WHERE state='APPROVED' GROUP BY specialist_id
) ratings ON ratings.specialist_id=eligible.id
WHERE ($1::text='' OR EXISTS (SELECT 1 FROM eligible selected WHERE selected.id=eligible.id AND selected.topic_id=$1::text))
 AND ($2::text='' OR eligible.id::text=$2::text)
GROUP BY eligible.id, eligible.display_name, eligible.profession_code,
 details.headline,details.about,details.experience_years,details.languages,details.limitations,
 ratings.review_count, ratings.rating_average
ORDER BY eligible.display_name, eligible.id
LIMIT $3 OFFSET $4`

func (c *Checker) PublicProfiles(ctx context.Context, topic string, limit, offset int) ([]specialist.PublicProfile, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	if topic != "" {
		normalized, err := specialist.NormalizeTopic(topic)
		if err != nil {
			return nil, err
		}
		topic = normalized
	}
	if limit < 1 || limit > 21 || offset < 0 || offset > 1000 {
		return nil, specialist.ErrPublicDetailsInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, publicSpecialistQuery, topic, "", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query public specialist directory: %w", err)
	}
	defer rows.Close()
	result := make([]specialist.PublicProfile, 0)
	for rows.Next() {
		profile, err := scanPublicProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public specialist directory: %w", err)
	}
	return result, nil
}
func (c *Checker) PublicProfile(ctx context.Context, specialistID string) (specialist.PublicProfile, error) {
	if c == nil || c.db == nil {
		return specialist.PublicProfile{}, errors.New("postgres checker is not initialized")
	}
	if len(specialistID) != 36 || strings.ContainsAny(specialistID, "/\\?&#") {
		return specialist.PublicProfile{}, specialist.ErrPublicProfileNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	row := c.db.QueryRowContext(ctx, publicSpecialistQuery, "", specialistID, 1, 0)
	result, err := scanPublicProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return specialist.PublicProfile{}, specialist.ErrPublicProfileNotFound
	}
	if err != nil {
		return specialist.PublicProfile{}, err
	}
	return result, nil
}

type publicRowScanner interface{ Scan(dest ...any) error }

func scanPublicProfile(row publicRowScanner) (specialist.PublicProfile, error) {
	var p specialist.PublicProfile
	var languagesJSON, limitationsJSON, topicsJSON string
	err := row.Scan(&p.ID, &p.DisplayName, &p.ProfessionCode, &p.Headline, &p.About,
		&p.ExperienceYears, &languagesJSON, &limitationsJSON, &topicsJSON,
		&p.VerifiedReviewCount, &p.VerifiedRating)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal([]byte(languagesJSON), &p.Languages); err != nil {
		return p, fmt.Errorf("decode public languages: %w", err)
	}
	if err = json.Unmarshal([]byte(limitationsJSON), &p.Limitations); err != nil {
		return p, fmt.Errorf("decode public limitations: %w", err)
	}
	if err = json.Unmarshal([]byte(topicsJSON), &p.PublishedTopics); err != nil {
		return p, fmt.Errorf("decode published topics: %w", err)
	}
	return p, nil
}
func (c *Checker) PublicDetails(ctx context.Context, identityID string) (specialist.PublicDetails, error) {
	if c == nil || c.db == nil {
		return specialist.PublicDetails{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	var p specialist.PublicDetails
	var languagesJSON, limitationsJSON string
	err := c.db.QueryRowContext(ctx, `SELECT COALESCE(details.headline,''), COALESCE(details.about,''),
  COALESCE(details.experience_years,0), COALESCE(to_json(details.languages)::text,'["ru"]'),
  COALESCE(to_json(details.limitations)::text,'[]')
  FROM specialist_profiles profile
  LEFT JOIN specialist_public_details details ON details.specialist_id=profile.id
  WHERE profile.identity_id::text=$1`, identityID).Scan(&p.Headline, &p.About, &p.ExperienceYears, &languagesJSON, &limitationsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return p, specialist.ErrProfileNotFound
	}
	if err != nil {
		return p, fmt.Errorf("read specialist public details: %w", err)
	}
	if err = json.Unmarshal([]byte(languagesJSON), &p.Languages); err != nil {
		return p, err
	}
	if err = json.Unmarshal([]byte(limitationsJSON), &p.Limitations); err != nil {
		return p, err
	}
	return p, nil
}
func (c *Checker) SavePublicDetails(ctx context.Context, identityID string, value specialist.PublicDetails) (specialist.PublicDetails, error) {
	if c == nil || c.db == nil {
		return specialist.PublicDetails{}, errors.New("postgres checker is not initialized")
	}
	p, err := specialist.NormalizePublicDetails(value)
	if err != nil {
		return specialist.PublicDetails{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return specialist.PublicDetails{}, err
	}
	defer tx.Rollback()
	var profileID string
	if err = tx.QueryRowContext(ctx, `SELECT id::text FROM specialist_profiles
 WHERE identity_id::text=$1 FOR UPDATE`, identityID).Scan(&profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return specialist.PublicDetails{}, specialist.ErrProfileNotFound
		}
		return specialist.PublicDetails{}, err
	}
	languagesJSON, _ := json.Marshal(p.Languages)
	limitationsJSON, _ := json.Marshal(p.Limitations)
	var changed bool
	err = tx.QueryRowContext(ctx, `
 WITH updated AS (
 INSERT INTO specialist_public_details(specialist_id,headline,about,experience_years,languages,limitations)
 VALUES ($1::uuid,$2,$3,$4,ARRAY(SELECT jsonb_array_elements_text($5::jsonb)),ARRAY(SELECT jsonb_array_elements_text($6::jsonb)))
 ON CONFLICT(specialist_id) DO UPDATE SET
 headline=EXCLUDED.headline,about=EXCLUDED.about,experience_years=EXCLUDED.experience_years,
 languages=EXCLUDED.languages,limitations=EXCLUDED.limitations,updated_at=now()
 WHERE (specialist_public_details.headline,specialist_public_details.about,specialist_public_details.experience_years,
  specialist_public_details.languages,specialist_public_details.limitations)
 IS DISTINCT FROM (EXCLUDED.headline,EXCLUDED.about,EXCLUDED.experience_years,EXCLUDED.languages,EXCLUDED.limitations)
 RETURNING specialist_id
 ) SELECT EXISTS(SELECT 1 FROM updated)`,
		profileID, p.Headline, p.About, p.ExperienceYears, string(languagesJSON), string(limitationsJSON)).Scan(&changed)
	if err != nil {
		return specialist.PublicDetails{}, fmt.Errorf("write specialist public details: %w", err)
	}
	if changed {
		// Review must approve these authored claims before they can appear publicly.
		// Existing publication is atomically suspended by canonical DB trigger.
		if _, err = tx.ExecContext(ctx, `UPDATE specialist_profiles SET review_state='PENDING',updated_at=now()
   WHERE id=$1::uuid AND review_state='APPROVED'`, profileID); err != nil {
			return specialist.PublicDetails{}, fmt.Errorf("invalidate specialist public claims review: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return specialist.PublicDetails{}, err
	}
	return p, nil
}


// RegisteredProfiles shows new, not-yet-reviewed registrations without
// publishing private identity/evidence, unreviewed biography or any offer.
// Previously published/revoked profiles are NOT reintroduced via this path.
func (c *Checker) RegisteredProfiles(ctx context.Context, topic string, limit int) ([]specialist.RegisteredProfile, error) {
 if c == nil || c.db == nil { return nil, errors.New("postgres checker is not initialized") }
 if topic != "" {
  normalized, err := specialist.NormalizeTopic(topic)
  if err != nil { return nil, err }
  topic = normalized
 }
 if limit < 1 || limit > 100 { return nil, specialist.ErrPublicDetailsInvalid }
 ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
 defer cancel()
 rows, err := c.db.QueryContext(ctx, `
 SELECT p.id::text, p.display_name, p.profession_code
 FROM specialist_profiles p
 WHERE p.profile_complete
   AND p.review_state <> 'REJECTED'
   AND NOT EXISTS (SELECT 1 FROM specialist_publications pub WHERE pub.specialist_id=p.id)
   AND ($1::text='' OR EXISTS (
     SELECT 1 FROM specialist_capabilities cap
     WHERE cap.specialist_id=p.id AND cap.topic_id=$1::text))
 ORDER BY p.display_name, p.id
 LIMIT $2`, topic, limit)
 if err != nil { return nil, fmt.Errorf("query registered specialists: %w", err) }
 defer rows.Close()
 result := make([]specialist.RegisteredProfile, 0)
 for rows.Next() {
  var p specialist.RegisteredProfile
  if err := rows.Scan(&p.ID, &p.DisplayName, &p.ProfessionCode); err != nil { return nil, err }
  p.VerificationStatus = "UNVERIFIED"
  result = append(result, p)
 }
 if err := rows.Err(); err != nil { return nil, err }
 return result, nil
}
