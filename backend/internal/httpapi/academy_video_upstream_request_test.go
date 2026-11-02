package httpapi

import (
	"context"
	"testing"
)

func TestAcademyVideoUpstreamRequestDoesNotForwardLearnerSecrets(t *testing.T) {
	request, err := academyVideoUpstreamRequest(context.Background(), "https://downloader.disk.yandex.net/video?download=1", "bytes=100-199")
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != "GET" || request.Header.Get("Range") != "bytes=100-199" {
		t.Fatal("incorrect media upstream request")
	}
	if request.Header.Get("Accept-Encoding") != "identity" {
		t.Fatal("upstream must request identity-encoded video bytes for stable range offsets")
	}
	for _, name := range []string{
		"Authorization", "Cookie", "Referer", "Forwarded", "X-Forwarded-For",
		"X-Forwarded-Host", "Proxy-Authorization", "Origin", "X-Api-Key",
	} {
		if request.Header.Get(name) != "" {
			t.Fatalf("sensitive browser header forwarded: %s", name)
		}
	}
}

func TestAcademyVideoUpstreamRequestRejectsUnsafeSourcesAndRanges(t *testing.T) {
	for _, tc := range []struct {
		name, source, requestedRange string
	}{
		{"loopback", "https://127.0.0.1/video", ""},
		{"untrusted host", "https://downloader.disk.yandex.net.evil.invalid/video", ""},
		{"insecure scheme", "http://downloader.disk.yandex.net/video", ""},
		{"embedded credentials", "https://token@downloader.disk.yandex.net/video", ""},
		{"multipart range", "https://downloader.disk.yandex.net/video", "bytes=0-1,3-4"},
		{"reversed range", "https://downloader.disk.yandex.net/video", "bytes=100-99"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := academyVideoUpstreamRequest(context.Background(), tc.source, tc.requestedRange); err == nil {
				t.Fatal("unsafe upstream request accepted")
			}
		})
	}
}

func TestAcademyVideoUpstreamRequestPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	request, err := academyVideoUpstreamRequest(ctx, "https://downloader.disk.yandex.net/video", "")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if request.Context().Err() != context.Canceled {
		t.Fatal("upstream request detached from revocable stream context")
	}
	if request.Header.Get("Range") != "" || request.Header.Get("Accept-Encoding") != "identity" {
		t.Fatal("full video request has unexpected representation headers")
	}
}
