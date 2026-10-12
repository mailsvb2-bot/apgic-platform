package specialist

import (
	"errors"
	"strings"
	"testing"
)

func TestPublicDetailsBoundsAndClaimSafety(t *testing.T) {
	good := PublicDetails{Headline: "Помогаю с тревогой", About: "Описание подхода", ExperienceYears: 8, Languages: []string{"ru", "EN"}, Limitations: []string{"Несовершеннолетние"}}
	normalized, err := NormalizePublicDetails(good)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Languages[1] != "en" || normalized.ExperienceYears != 8 {
		t.Fatalf("normalized=%#v", normalized)
	}
	cases := []PublicDetails{
		{Headline: strings.Repeat("a", 121)}, {About: strings.Repeat("a", 3001)},
		{ExperienceYears: -1}, {ExperienceYears: 76}, {Headline: "bad\x00data"},
		{Languages: []string{"xx"}}, {Languages: []string{""}},
		{Limitations: []string{strings.Repeat("a", 181)}},
	}
	for i, p := range cases {
		if _, err := NormalizePublicDetails(p); !errors.Is(err, ErrPublicDetailsInvalid) {
			t.Errorf("case %d: expected invalid, got %v", i, err)
		}
	}
}
