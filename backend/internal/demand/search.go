package demand

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

var ErrSpecialistSupplyUnavailable = errors.New("trusted specialist supply is unavailable")

const (
	searchVersion = "search-projection-v1"
	searchNotice  = "Поиск — проекция каталога. Индекс не владеет допуском и квалификацией."
	staleNotice   = "Индекс устарел: часть специалистов в нём пропала. Допуск в каталоге не изменился."
)

type PublishedSpecialistStore interface {
	PublishedProfiles() ([]marketplace.SpecialistProfile, error)
}

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
	// Public reads must see current PostgreSQL review/publication state.
	// Never hold the global demand mutex across external storage I/O: that
	// would serialize public searches with booking, payment and consultation.
	if s.specialistStore != nil {
		s.mu.Lock()
		profiles := make([]marketplace.SpecialistProfile, 0, len(s.catalog.candidates))
		for _, candidate := range s.catalog.candidates {
			profiles = append(profiles, candidate.Profile)
		}
		s.mu.Unlock()

		durable, err := s.specialistStore.PublishedProfiles()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSpecialistSupplyUnavailable, err)
		}
		profiles = append(profiles, durable...)
		// This is a request-local rebuildable projection, not new canonical
		// truth. Concurrent responses cannot overwrite a newer shared cache.
		projection := marketplace.RebuildSearchProjection(searchVersion, profiles)
		return searchView(projection, topic, false, false), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProjectionLocked(); err != nil {
		return nil, err
	}
	return s.searchViewLocked(topic, false), nil
}

func (s *Service) RebuildSearch(topic string) (*SearchView, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, ErrTopicRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rebuildProjectionLocked(); err != nil {
		return nil, err
	}
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
	if err := s.ensureProjectionLocked(); err != nil {
		return nil, err
	}
	s.projection = s.projection.Drop(specialistID)
	s.searchStale = true
	return s.searchViewLocked(topic, false), nil
}

func (s *Service) ensureProjectionLocked() error {
	if s.projection.Version == "" {
		return s.rebuildProjectionLocked()
	}
	return nil
}

func (s *Service) rebuildProjectionLocked() error {
	profiles := make([]marketplace.SpecialistProfile, 0, len(s.catalog.candidates))
	for _, candidate := range s.catalog.candidates {
		profiles = append(profiles, candidate.Profile)
	}
	if s.specialistStore != nil {
		durable, err := s.specialistStore.PublishedProfiles()
		if err != nil {
			return err
		}
		profiles = append(profiles, durable...)
	}
	s.projection = marketplace.RebuildSearchProjection(searchVersion, profiles)
	s.searchStale = false
	return nil
}

func (s *Service) searchViewLocked(topic string, rebuilt bool) *SearchView {
	return searchView(s.projection, topic, rebuilt, s.searchStale)
}

func searchView(projection marketplace.SearchProjection, topic string, rebuilt, stale bool) *SearchView {
	entries := projection.SearchTopic(topic)
	hits := make([]SearchHit, 0, len(entries))
	for _, entry := range entries {
		hits = append(hits, SearchHit{
			SpecialistID: entry.SpecialistID,
			DisplayName:  entry.DisplayName,
			Topics:       entry.Topics,
		})
	}
	notice := searchNotice
	if stale {
		notice = staleNotice
	}
	return &SearchView{
		Version:           projection.Version,
		Topic:             topic,
		Rebuilt:           rebuilt,
		Stale:             stale,
		OwnsQualification: false,
		Entries:           hits,
		Notice:            notice,
	}
}
