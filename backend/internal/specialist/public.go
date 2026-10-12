package specialist

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

var ErrPublicProfileNotFound = errors.New("public specialist profile not found")
var ErrPublicDetailsInvalid = errors.New("public specialist details invalid")

// PublicDetails contains only specialist-authored, reviewable presentation data.
// Identity details and private competence evidence never enter the public DTO.
type PublicDetails struct {
	Headline        string   `json:"headline"`
	About           string   `json:"about"`
	ExperienceYears int      `json:"experience_years"`
	Languages       []string `json:"languages"`
	Limitations     []string `json:"limitations"`
}
type PublicProfile struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"display_name"`
	ProfessionCode  string   `json:"profession_code"`
	Headline        string   `json:"headline"`
	About           string   `json:"about"`
	ExperienceYears int      `json:"experience_years"`
	Languages       []string `json:"languages"`
	Limitations     []string `json:"limitations"`
	PublishedTopics []string `json:"published_topics"`
	VerifiedReviewCount int `json:"verified_review_count"`
	VerifiedRating *float64 `json:"verified_rating,omitempty"`
}
type PublicDirectoryStore interface {
	PublicProfile(ctx context.Context, specialistID string) (PublicProfile, error)
	PublicProfiles(ctx context.Context, topic string, limit, offset int) ([]PublicProfile, error)
	PublicDetails(ctx context.Context, identityID string) (PublicDetails, error)
	SavePublicDetails(ctx context.Context, identityID string, value PublicDetails) (PublicDetails, error)
}

// NormalizePublicDetails guards bounded, display-only content. Evidence, verification,
// ratings and booking availability are deliberately not author-editable here.
func NormalizePublicDetails(value PublicDetails) (PublicDetails, error) {
	value.Headline = strings.TrimSpace(value.Headline)
	value.About = strings.TrimSpace(value.About)
	if len([]rune(value.Headline)) > 120 || len([]rune(value.About)) > 3000 ||
		value.ExperienceYears < 0 || value.ExperienceYears > 75 ||
		!safePublicText(value.Headline) || !safePublicText(value.About) {
		return PublicDetails{}, ErrPublicDetailsInvalid
	}
	languages, ok := normalizePublicList(value.Languages, 8, 16)
	if !ok {
		return PublicDetails{}, ErrPublicDetailsInvalid
	}
	if len(languages) == 0 {
		languages = []string{"ru"}
	}
	for i, lang := range languages {
		lang = strings.ToLower(lang)
		if lang != "ru" && lang != "en" && lang != "de" && lang != "fr" && lang != "es" && lang != "it" && lang != "pt" && lang != "zh" && lang != "ar" && lang != "kk" && lang != "uz" {
			return PublicDetails{}, ErrPublicDetailsInvalid
		}
		languages[i] = lang
	}
	value.Languages = languages
	limitations, ok := normalizePublicList(value.Limitations, 12, 180)
	if !ok {
		return PublicDetails{}, ErrPublicDetailsInvalid
	}
	value.Limitations = limitations
	return value, nil
}
func normalizePublicList(list []string, maxCount, maxLength int) ([]string, bool) {
	if len(list) > maxCount {
		return nil, false
	}
	result := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, raw := range list {
		s := strings.TrimSpace(raw)
		if s == "" || len([]rune(s)) > maxLength || !safePublicText(s) {
			return nil, false
		}
		key := strings.ToLower(s)
		if !seen[key] {
			result = append(result, s)
			seen[key] = true
		}
	}
	return result, true
}
func safePublicText(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}
