package academy

import (
	"strings"
	"unicode/utf8"
)

// ValidCourseTitle agrees with PostgreSQL length(text) and the Academy
// editor's 1..200-character contract. A byte-count check incorrectly
// rejects valid Cyrillic, CJK and other multibyte author titles.
func ValidCourseTitle(title string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(title))
	return n >= 1 && n <= 200
}
