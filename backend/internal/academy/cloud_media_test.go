package academy

import "testing"

func TestValidYandexDiskPathKeepsVerifiedIdentityExact(t *testing.T) {
	for _, path := range []string{
		"disk:/lecture.mp4",
		"disk:/nested/лекция 1.mp4",
	} {
		if !ValidYandexDiskPath(path) {
			t.Fatalf("valid private path rejected: %q", path)
		}
	}
	for _, path := range []string{
		"",
		"https://example.org/lecture.mp4",
		"disk:",
		"disk:/lecture.mp4 ",
		" disk:/lecture.mp4",
		"disk:/lecture.mp4\n",
		"disk:/lecture.mp4\x00",
		"disk:/" + string(make([]byte, 2048)),
	} {
		if ValidYandexDiskPath(path) {
			t.Fatalf("noncanonical private path accepted: %q", path)
		}
	}
}
