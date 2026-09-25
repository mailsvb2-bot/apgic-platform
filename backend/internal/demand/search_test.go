package demand

import "testing"

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
