package academy

import "strings"

// ValidYandexDiskPath enforces the exact identity used for provider
// verification, catalog persistence and subsequent protected playback.
// Normalizing after verification could silently point at a different file.
func ValidYandexDiskPath(path string) bool {
	return strings.HasPrefix(path, "disk:/") && len(path) <= 2048 &&
		strings.TrimSpace(path) == path && !strings.ContainsRune(path, '\x00')
}

type CloudMedia struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	ProviderFileRef string `json:"provider_file_ref"`
	SourceURL       string `json:"source_url,omitempty"`
	Title           string `json:"title"`
	Active          bool   `json:"active"` // author-only: currently verified OAuth generation
	ResourceID      string `json:"-"`      // verified server-side, never accepted from JSON
	SHA256          string `json:"-"`      // immutable provider content digest
	FileSize        int64  `json:"-"`
}
type MediaAssignment struct {
	LessonID string `json:"lesson_id"`
	MediaID  string `json:"media_id"`
	Ordinal  int    `json:"ordinal"`
}
type LessonMediaView struct {
	LessonID        string `json:"lesson_id"`
	MediaID         string `json:"media_id"`
	Ordinal         int    `json:"ordinal"`
	Title           string `json:"title"`
	Provider        string `json:"provider"`
	ProviderFileRef string `json:"provider_file_ref"`
	SourceURL       string `json:"source_url,omitempty"`
}
