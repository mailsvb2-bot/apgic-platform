package academy

type CloudMedia struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	ProviderFileRef string `json:"provider_file_ref"`
	SourceURL       string `json:"source_url,omitempty"`
	Title           string `json:"title"`
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
