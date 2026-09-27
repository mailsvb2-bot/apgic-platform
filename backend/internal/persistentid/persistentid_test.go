package persistentid

import (
	"regexp"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[45][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewReturnsUUIDv4(t *testing.T) {
	first, err := New()
	if err != nil {
		t.Fatal(err)
	}
	second, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("random persistent ids must differ")
	}
	if !uuidPattern.MatchString(first) || first[14] != '4' {
		t.Fatalf("not UUIDv4: %s", first)
	}
}

func TestFromRefIsStableScopedUUIDv5(t *testing.T) {
	first, err := FromRef("catalog-slot", "spec-lebedeva-slot-1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := FromRef("catalog-slot", "spec-lebedeva-slot-1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := FromRef("catalog-specialist", "spec-lebedeva-slot-1")
	if err != nil {
		t.Fatal(err)
	}
	if first != again || first == other {
		t.Fatalf("determinism/scope mismatch first=%s again=%s other=%s", first, again, other)
	}
	if !uuidPattern.MatchString(first) || first[14] != '5' {
		t.Fatalf("not UUIDv5-shaped: %s", first)
	}
}

func TestFromRefRejectsBlankInputs(t *testing.T) {
	if _, err := FromRef("", "x"); err == nil {
		t.Fatal("blank scope accepted")
	}
	if _, err := FromRef("slot", " "); err == nil {
		t.Fatal("blank ref accepted")
	}
}
