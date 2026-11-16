package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// academyVideoUpstreamRequest builds a fresh, credential-free request.
// The learner's cookies, Authorization, Referer and forwarding headers must
// never cross the APGIC -> Yandex media trust boundary.
func academyVideoUpstreamRequest(ctx context.Context, sourceURL, requestedRange string) (*http.Request, error) {
	if !academy.ValidateYandexDownloadURL(sourceURL) {
		return nil, errors.New("untrusted video source")
	}
	if requestedRange != "" && !academyVideoSingleRange(requestedRange) {
		return nil, errors.New("invalid video range")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	// A byte range is defined over the identity representation. Explicitly
	// decline content encoding so cloud/CDN compression cannot alter offsets.
	request.Header.Set("Accept-Encoding", "identity")
	if requestedRange != "" {
		request.Header.Set("Range", requestedRange)
	}
	return request, nil
}
