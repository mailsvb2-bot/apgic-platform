package organization

import "testing"

func TestDirectionUsesArchiveInsteadOfHardDelete(t *testing.T) {
	org := New("org-1", "Example")
	org.AddDirection("dir-1", "Consulting")
	org.Directions["dir-1"].HasDependentTruth = true

	if err := org.HardDeleteDirection("dir-1"); err != ErrHardDeleteForbidden {
		t.Fatalf("hard delete must be blocked, got %v", err)
	}
	if _, exists := org.Directions["dir-1"]; !exists {
		t.Fatal("hard delete removed direction")
	}
	if err := org.ArchiveDirection("dir-1"); err != nil {
		t.Fatal(err)
	}
	if org.Directions["dir-1"].Status != DirectionArchived {
		t.Fatal("archive must preserve direction and its historical links")
	}
}


func TestDirectionTypeIsUniversalAndNormalized(t *testing.T) {
	name, directionType, err := NormalizeDirection(" Rehabilitation ", " rehabilitation ")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Rehabilitation" || directionType != "REHABILITATION" {
		t.Fatalf("normalized direction=(%q,%q)", name, directionType)
	}

	org := New("org-1", "Example")
	org.AddTypedDirection("dir-1", name, directionType)
	if org.Directions["dir-1"].Type != "REHABILITATION" {
		t.Fatalf("direction type=%q", org.Directions["dir-1"].Type)
	}
}

func TestOrganizationValidationRejectsBlankValues(t *testing.T) {
	if _, err := NormalizeOrganizationName("   "); err != ErrInvalidOrganization {
		t.Fatalf("blank organization error=%v", err)
	}
	if _, _, err := NormalizeDirection("Direction", "  "); err != ErrInvalidDirection {
		t.Fatalf("blank direction type error=%v", err)
	}
}
