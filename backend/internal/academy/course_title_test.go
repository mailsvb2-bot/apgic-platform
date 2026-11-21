package academy

import (
	"strings"
	"testing"
)

func TestValidCourseTitleMatchesPostgresCharacterLength(t *testing.T) {
	cases := []struct {
		name, title string
		want bool
	}{
		{"empty", "", false},
		{"whitespace", " \t\n ", false},
		{"single character", "1", true},
		{"numbered course", "№1", true},
		{"short Cyrillic", "Я", true},
		{"200 Cyrillic characters", strings.Repeat("я", 200), true},
		{"201 Cyrillic characters", strings.Repeat("я", 201), false},
		{"200 CJK characters", strings.Repeat("学", 200), true},
		{"201 ASCII characters", strings.Repeat("a", 201), false},
		{"200 ASCII plus whitespace", "  " + strings.Repeat("a", 200) + "  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidCourseTitle(tc.title); got != tc.want {
				t.Fatalf("ValidCourseTitle(%q) = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}
