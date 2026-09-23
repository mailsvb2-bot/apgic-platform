package organization

import "testing"

func TestDirectionWithHistoricalTruthCannotBeHardDeleted(t *testing.T) {
	org := New("org-1", "Example")
	org.AddDirection("dir-1", "Consulting")
	org.Directions["dir-1"].HasDependentTruth = true

	if err := org.HardDeleteDirection("dir-1"); err != ErrDependentTruth {
		t.Fatalf("expected dependent truth guard, got %v", err)
	}
	if err := org.ArchiveDirection("dir-1"); err != nil {
		t.Fatal(err)
	}
	if org.Directions["dir-1"].Status != DirectionArchived {
		t.Fatal("archive must preserve direction and its historical links")
	}
}
