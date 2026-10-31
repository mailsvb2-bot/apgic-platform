package academy

type NotificationPreferences struct {
	Enabled            bool `json:"enabled"`
	LearningUpdates    bool `json:"learning_updates"`
	AssessmentResults  bool `json:"assessment_results"`
	CertificateUpdates bool `json:"certificate_updates"`
}
