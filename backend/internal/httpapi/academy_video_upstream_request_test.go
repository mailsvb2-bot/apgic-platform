package httpapi

import (
	"context"
	"testing"
)

func TestAcademyVideoUpstreamRequestDoesNotForwardLearnerSecrets(t *testing.T) {
	request, err := academyVideoUpstreamRequest(context.Background(), "https://downloader.disk.yandex.net/video?download=1", "bytes=100-199")
	if err != nil { t.Fatal(err) }
	if request.Method != "GET" || request.Header.Get("Range") != "bytes=100-199" { t.Fatal("incorrect media upstream request") }
	for _, name := range []string{"Authorization", "Cookie", "Referer", "X-Forwarded-For", "Proxy-Authorization"} {
		if request.Header.Get(name) != "" { t.Fatalf("sensitive browser header forwarded: %s", name) }
	}
	if _, err := academyVideoUpstreamRequest(context.Background(), "https://127.0.0.1/video", ""); err == nil { t.Fatal("localhost accepted") }
	if _, err := academyVideoUpstreamRequest(context.Background(), "https://downloader.disk.yandex.net/video", "bytes=0-1,3-4"); err == nil { t.Fatal("multipart range accepted") }
}
