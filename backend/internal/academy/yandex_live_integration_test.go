package academy

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Live provider smoke proof is opt-in: CI without a connected test account
// cannot be used as evidence of real Yandex media playback. The OAuth token
// and temporary download URL are never logged or returned to the browser.
func TestLiveYandexVerifiedVideoRange(t *testing.T) {
	token := os.Getenv("APGIC_TEST_YANDEX_OAUTH_TOKEN")
	path := os.Getenv("APGIC_TEST_YANDEX_VIDEO_PATH")
	if token == "" || path == "" {
		t.Skip("live Yandex OAuth token and owned test video are required")
	}
	if !ValidYandexDiskPath(path) {
		t.Fatal("invalid test video path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	provider := YandexDiskClient{}
	file, err := provider.VerifyVideo(ctx, token, path)
	if err != nil {
		t.Fatal("live Yandex metadata verification failed")
	}
	if file.Path != path || file.Size <= 0 || file.ResourceID == "" ||
		!validYandexSHA256(file.SHA256) {
		t.Fatal("live Yandex file identity is incomplete")
	}

	href, err := provider.TemporaryDownload(ctx, token, path)
	if err != nil || !ValidateYandexDownloadURL(href) {
		t.Fatal("live Yandex temporary video URL is unavailable or untrusted")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
	if err != nil {
		t.Fatal("unable to build live video range request")
	}
	request.Header.Set("Range", "bytes=0-1023")
	request.Header.Set("Accept-Encoding", "identity")
	client := NewYandexSecureClient(20 * time.Second)
	client.CheckRedirect = YandexMediaCheckRedirect
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("live Yandex video range transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("live Yandex video seek unsupported: HTTP %d", response.StatusCode)
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		t.Fatal("range response is content-encoded")
	}
	rangeHeader := response.Header.Get("Content-Range")
	if !strings.HasPrefix(rangeHeader, "bytes 0-") {
		t.Fatal("invalid partial video range metadata")
	}
	parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes 0-"), "/")
	if len(parts) != 2 {
		t.Fatal("invalid content-range representation")
	}
	last, errLast := strconv.ParseInt(parts[0], 10, 64)
	total, errTotal := strconv.ParseInt(parts[1], 10, 64)
	if errLast != nil || errTotal != nil || total != file.Size || last < 0 || last >= 1024 || last >= total {
		t.Fatal("cloud video range size disagrees with verified file")
	}
	// Do not download entire source; this test needs only the first bounded
	// range to prove that a real authorized player can start seeking.
	got, err := io.Copy(io.Discard, io.LimitReader(response.Body, 1025))
	if err != nil || got != last+1 {
		t.Fatal("live video yielded incomplete or excessive first range")
	}
}
