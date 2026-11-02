package academy

import (
	"context"
	"strings"
	"testing"
)

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
		"disk:/" + strings.Repeat("x", 2048),
	} {
		if ValidYandexDiskPath(path) {
			t.Fatalf("noncanonical private path accepted: %q", path)
		}
	}
}

func TestYandexDiskProviderRejectsChangedFileIdentityBeforeNetwork(t *testing.T) {
	client := YandexDiskClient{}
	for _, path := range []string{"disk:/lecture.mp4 ", " disk:/lecture.mp4", "disk:/lecture.mp4\x00"} {
		if _, err := client.VerifyVideo(context.Background(), "private-token", path); err == nil {
			t.Fatalf("provider accepted uncanonical video identity %q", path)
		}
		if _, err := client.TemporaryDownload(context.Background(), "private-token", path); err == nil {
			t.Fatalf("provider accepted uncanonical download identity %q", path)
		}
	}
}
