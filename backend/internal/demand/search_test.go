package demand

import (
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

func TestSearchRebuildRestoresCatalogWithoutChangingQualification(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	found, err := service.Search("sleep")
	if err != nil || found.OwnsQualification || found.Stale || len(found.Entries) != 1 || found.Entries[0].SpecialistID != "spec-lebedeva" {
		t.Fatalf("search = %#v err=%v", found, err)
	}
	stale, err := service.MarkSearchStale("sleep", "spec-lebedeva")
	if err != nil || !stale.Stale || len(stale.Entries) != 0 {
		t.Fatalf("stale = %#v err=%v", stale, err)
	}
	matches, topic, err := service.Matches(intent.ID, "sleep")
	if err != nil || topic != "sleep" || len(matches) != 1 || matches[0].SpecialistID != "spec-lebedeva" {
		t.Fatalf("qualification changed topic=%s matches=%#v err=%v", topic, matches, err)
	}
	rebuilt, err := service.RebuildSearch("sleep")
	if err != nil || !rebuilt.Rebuilt || rebuilt.Stale || rebuilt.OwnsQualification || len(rebuilt.Entries) != 1 {
		t.Fatalf("rebuilt = %#v err=%v", rebuilt, err)
	}
	if rebuilt.Entries[0].DisplayName != "Марина Лебедева" {
		t.Fatalf("name = %s", rebuilt.Entries[0].DisplayName)
	}
}


type publishedSpecialistStoreStub struct {
	profiles []marketplace.SpecialistProfile
}

func (s publishedSpecialistStoreStub) PublishedProfiles() ([]marketplace.SpecialistProfile, error) {
	return append([]marketplace.SpecialistProfile(nil), s.profiles...), nil
}

func TestSearchProjectionIncludesDurablePublishedSpecialists(t *testing.T) {
	profile, err := marketplace.NewSpecialistProfile("durable-specialist", "identity-durable", "Дарья Профи")
	if err != nil {
		t.Fatal(err)
	}
	capability, err := marketplace.NewCapability("anxiety", marketplace.EvidenceAPGICVerified, "evidence:durable")
	if err != nil {
		t.Fatal(err)
	}
	profile.AddCapability(capability)
	profile.Profession = "PSYCHOLOGIST"
	profile.Complete = true
	profile.Review = marketplace.ReviewApproved
	profile.PublishState = marketplace.PublishPublished
	profile.PublishedTopics = []string{"anxiety"}

	service, err := NewConformanceServiceWithStoresAndSpecialists(
		nil,
		nil,
		nil,
		publishedSpecialistStoreStub{profiles: []marketplace.SpecialistProfile{*profile}},
	)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.RebuildSearch("anxiety")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range view.Entries {
		if entry.SpecialistID == "durable-specialist" {
			found = true
			if entry.DisplayName != "Дарья Профи" {
				t.Fatalf("durable specialist display name = %q", entry.DisplayName)
			}
		}
	}
	if !found {
		t.Fatalf("durable published specialist missing from search projection: %#v", view.Entries)
	}
}
