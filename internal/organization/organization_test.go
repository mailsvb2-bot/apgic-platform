package organization

import "testing"

func TestDirectionIsArchivedInsteadOfDeleted(t *testing.T) {
	org, err := New("org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := org.AddDirection("dir-1", "ANY_SUPPORTED_ACTIVITY"); err != nil {
		t.Fatal(err)
	}
	if err := org.ArchiveDirection("dir-1"); err != nil {
		t.Fatal(err)
	}
	direction, ok := org.Direction("dir-1")
	if !ok {
		t.Fatal("direction truth was deleted")
	}
	if direction.State != DirectionArchived {
		t.Fatalf("unexpected state: %s", direction.State)
	}
}
