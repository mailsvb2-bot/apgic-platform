package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

const (
	searchVersion = "search-projection-v1"
	searchNotice  = "Поиск — проекция каталога. Индекс не владеет допуском и квалификацией."
	staleNotice   = "Индекс устарел: часть специалистов в нём пропала. Допуск в каталоге не изменился."
)

type SearchHit struct {
	SpecialistID string   `json:"specialist_id"`
	DisplayName  string   `json:"display_name"`
	Topics       []string `json:"topics"`
}

type SearchView struct {
	Version           string      `json:"version"`
	Topic             string      `json:"topic,omitempty"`
	Rebuilt           bool        `json:"rebuilt"`
	Stale             bool        `json:"stale"`
	OwnsQualification bool        `json:"owns_qualification"`
	Entries           []SearchHit `json:"entries"`
	Notice            string      `json:"notice"`
}

func (s *Service) Search(topic string) (*SearchView, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, ErrTopicRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureProjectionLocked()
	return s.searchViewLocked(topic, false), nil
}

func (s *Service) RebuildSearch(topic string) (*SearchView, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, ErrTopicRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuildProjectionLocked()
	view := s.searchViewLocked(topic, true)
	return view, nil
}

func (s *Service) MarkSearchStale(topic, specialistID string) (*SearchView, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, ErrTopicRequired
	}
	if strings.TrimSpace(specialistID) == "" {
		return nil, ErrSpecialistNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureProjectionLocked()
	s.projection = s.projection.Drop(specialistID)
	s.searchStale = true
	return s.searchViewLocked(topic, false), nil
}

func (s *Service) ensureProjectionLocked() {
	if s.projection.Version == "" {
		s.rebuildProjectionLocked()
	}
}

func (s *Service) rebuildProjectionLocked() {
	profiles := make([]marketplace.SpecialistProfile, 0, len(s.catalog.candidates))
	for _, candidate := range s.catalog.candidates {
		profiles = append(profiles, candidate.Profile)
	}
	s.projection = marketplace.RebuildSearchProjection(searchVersion, profiles)
	s.searchStale = false
}

func (s *Service) searchViewLocked(topic string, rebuilt bool) *SearchView {
	entries := s.projection.SearchTopic(topic)
	hits := make([]SearchHit, 0, len(entries))
	for _, entry := range entries {
		hits = append(hits, SearchHit{
			SpecialistID: entry.SpecialistID,
			DisplayName:  entry.DisplayName,
			Topics:       entry.Topics,
		})
	}
	notice := searchNotice
	if s.searchStale {
		notice = staleNotice
	}
	return &SearchView{
		Version:           s.projection.Version,
		Topic:             topic,
		Rebuilt:           rebuilt,
		Stale:             s.searchStale,
		OwnsQualification: false,
		Entries:           hits,
		Notice:            notice,
	}
}
