package academy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// YandexDiskClient is a server-side transport. The OAuth token must never
// reach browser JSON, lesson content or a log message.
type YandexDiskClient struct {
	Client *http.Client
}

type YandexDiskFile struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	ResourceID string `json:"resource_id"`
	SHA256     string `json:"sha256"`
}
type yandexDiskFilesResponse struct {
	Embedded *struct {
		Items *[]YandexDiskFile `json:"items"`
		Total *int              `json:"total"`
	} `json:"_embedded"`
}
type yandexDiskDownloadResponse struct {
	Href      string `json:"href"`
	Method    string `json:"method"`
	Templated bool   `json:"templated"`
}

const yandexDiskAPI = "https://cloud-api.yandex.net/v1/disk/resources"

// Fixed errors permit diagnosis without logging HTTP requests, tokens or paths.
var (
	ErrYandexTransport       = errors.New("cloud transport failed")
	ErrYandexJSON            = errors.New("cloud provider JSON invalid")
	ErrYandexListing         = errors.New("cloud provider listing invalid")
	ErrYandexReauth          = errors.New("cloud provider authorization rejected")
	ErrYandexResourceMissing = errors.New("cloud provider resource not found")
	ErrYandexRateLimited     = errors.New("cloud provider rate limited")
	ErrYandexProviderDown    = errors.New("cloud provider unavailable")
)

func (c YandexDiskClient) do(ctx context.Context, token, endpoint string, dst any) error {
	if token == "" || len(token) > 8192 {
		return errors.New("missing cloud authorization")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Accept", "application/json")
	client := c.Client
	if client == nil {
		// The default http.Client inherits environment proxies. That could
		// disclose the author's OAuth token to an untrusted proxy and
		// bypass the gateway's public-IP SSRF boundary.
		client = NewYandexSecureClient(15 * time.Second)
	}
	// A provider redirect must never receive the author's OAuth credential.
	// Clone the client to enforce this even when an injected client permits redirects.
	protected := *client
	protected.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	var resp *http.Response
	for attempt := 1; ; attempt++ {
		resp, err = protected.Do(req)
		wait, retry := time.Duration(0), false
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return context.DeadlineExceeded
			}
			if ctx.Err() != nil {
				return ErrYandexTransport
			}
			retry = true
		} else if yandexRetryableStatus(resp.StatusCode) {
			retry = true
			wait = yandexRetryAfter(resp.Header.Get("Retry-After"))
		}
		if !retry || attempt >= yandexMaxAttempts {
			break
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			resp = nil
		}
		if wait == 0 {
			wait = yandexRetryBaseDelay << (attempt - 1)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return context.DeadlineExceeded
			}
			return ErrYandexTransport
		case <-timer.C:
		}
	}
	if err != nil {
		return ErrYandexTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return YandexStatusError{Status: resp.StatusCode}
	}
	// Decode one bounded JSON document, not merely a valid prefix followed
	// by another object or malformed provider data.
	decoder := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 2<<20))
	if err := decoder.Decode(dst); err != nil {
		return ErrYandexJSON
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrYandexJSON
	}
	return nil
}

// ListFiles uses bounded pagination: a browser can batch-select an entire
// folder while the API never returns the secret token.
// Ask Yandex for the complete _embedded object: requesting only nested
// projections can omit pagination metadata on some provider responses.
// The decoder still exposes ONLY the allowlisted YandexDiskFile fields.
func (c YandexDiskClient) ListFiles(ctx context.Context, token, path string, offset, limit int) ([]YandexDiskFile, int, error) {
	if !ValidYandexDiskPath(path) || offset < 0 || offset > 1000000 || limit < 1 || limit > 1000 {
		return nil, 0, errors.New("invalid cloud file request")
	}
	q := url.Values{"path": {path}, "offset": {fmt.Sprint(offset)}, "limit": {fmt.Sprint(limit)}, "fields": {"_embedded"}}
	var d yandexDiskFilesResponse
	if err := c.do(ctx, token, yandexDiskAPI+"?"+q.Encode(), &d); err != nil {
		return nil, 0, err
	}
	// Missing or null listing fields are not an empty folder. Without
	// presence checks, Go zero-values silently turn malformed 200 responses
	// into a successful empty listing, hiding provider failures.
	if d.Embedded == nil || d.Embedded.Items == nil || d.Embedded.Total == nil {
		return nil, 0, fmt.Errorf("%w: missing_fields", ErrYandexListing)
	}
	items, total := *d.Embedded.Items, *d.Embedded.Total
	// Provider metadata is not an authorization decision. Reject malformed
	// folder entries before they become browser navigation/import candidates.
	// Only direct children of the requested folder may be displayed.
	if total < 0 || len(items) > limit ||
		(len(items) > 0 && (offset > total || len(items) > total-offset)) {
		return nil, 0, fmt.Errorf("%w: pagination", ErrYandexListing)
	}
	prefix := strings.TrimSuffix(path, "/") + "/"
	seen := make(map[string]struct{}, len(items))
	for _, file := range items {
		if !ValidYandexDiskPath(file.Path) || !strings.HasPrefix(file.Path, prefix) ||
			(file.Type != "file" && file.Type != "dir") || file.Name == "" || file.Size < 0 {
			return nil, 0, fmt.Errorf("%w: entry", ErrYandexListing)
		}
		child := strings.TrimPrefix(file.Path, prefix)
		if child == "" || strings.Contains(child, "/") || file.Name != child {
			return nil, 0, fmt.Errorf("%w: entry", ErrYandexListing)
		}
		// A duplicate path or a display name inconsistent with the verified
		// provider path can mislead the author into selecting another asset.
		if _, exists := seen[file.Path]; exists {
			return nil, 0, fmt.Errorf("%w: duplicate", ErrYandexListing)
		}
		seen[file.Path] = struct{}{}
	}
	return items, total, nil
}

// VerifyVideo resolves a selected file using the author's OAuth credential
// before accepting it into the private Academy catalog. Client-supplied names,
// paths and MIME types are never proof that the file exists or is playable.
func (c YandexDiskClient) VerifyVideo(ctx context.Context, token, path string) (YandexDiskFile, error) {
	if !ValidYandexDiskPath(path) {
		return YandexDiskFile{}, errors.New("invalid cloud video path")
	}
	q := url.Values{"path": {path}, "fields": {"name,path,type,mime_type,size,resource_id,sha256"}}
	return c.verifyVideoAt(ctx, token, path, yandexDiskAPI+"?"+q.Encode())
}

// The private helper allows a real HTTP test server to exercise the exact
// provider response validation without contacting a production OAuth account.
func (c YandexDiskClient) verifyVideoAt(ctx context.Context, token, path, endpoint string) (YandexDiskFile, error) {
	var file YandexDiskFile
	if err := c.do(ctx, token, endpoint, &file); err != nil {
		return YandexDiskFile{}, err
	}
	if file.Path != path || file.Type != "file" || file.Size <= 0 ||
		!strings.HasPrefix(strings.ToLower(file.MimeType), "video/") ||
		len(file.ResourceID) == 0 || len(file.ResourceID) > 512 ||
		strings.TrimSpace(file.ResourceID) != file.ResourceID ||
		!validYandexSHA256(file.SHA256) {
		return YandexDiskFile{}, errors.New("cloud file is not a verified video")
	}
	return file, nil
}

// A provider path alone is mutable. Require both the provider resource ID
// and the full content digest to bind an import to the exact verified file.
func validYandexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// TemporaryDownload is used ONLY server-side by the media gateway.
// The result must never be included in an author or learner API JSON response.
func (c YandexDiskClient) TemporaryDownload(ctx context.Context, token, path string) (string, error) {
	if !ValidYandexDiskPath(path) {
		return "", errors.New("invalid disk path")
	}
	q := url.Values{"path": {path}}
	var d yandexDiskDownloadResponse
	if err := c.do(ctx, token, yandexDiskAPI+"/download?"+q.Encode(), &d); err != nil {
		return "", err
	}
	if !ValidateYandexDownloadURL(d.Href) || d.Method != "GET" || d.Templated {
		return "", errors.New("invalid download address")
	}
	return d.Href, nil
}

// Restrict provider-supplied download links to the documented downloader hosts.
// Broad suffix matches would permit arbitrary hosts beneath disk.yandex.net.
func trustedYandexDownloadHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "downloader.disk.yandex.net" || host == "downloader.disk.yandex.ru"
}

// ValidateYandexDownloadURL blocks SSRF via malicious provider hrefs.
// The downloader is always contacted without following redirects.
func ValidateYandexDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Fragment == "" &&
		(u.Port() == "" || u.Port() == "443") && trustedYandexDownloadHost(u.Hostname())
}

// Yandex downloader can redirect to its HTTPS storage node.
func trustedYandexStorageHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if trustedYandexDownloadHost(host) {
		return true
	}
	label, ok := strings.CutSuffix(host, ".storage.yandex.net")
	return ok && label != "" && !strings.Contains(label, ".")
}

// ValidateYandexStorageRedirect validates a provider redirect hop.
func ValidateYandexStorageRedirect(target *url.URL) bool {
	return target != nil && target.Scheme == "https" && target.User == nil &&
		(target.Port() == "" || target.Port() == "443") && trustedYandexStorageHost(target.Hostname())
}

const YandexMediaMaxRedirects = 3

// Reject untrusted destinations and strip credentials on redirect.
func YandexMediaCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || len(via) > YandexMediaMaxRedirects || req == nil || !ValidateYandexStorageRedirect(req.URL) {
		return http.ErrUseLastResponse
	}
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
		req.Header.Del(name)
	}
	return nil
}

type YandexStatusError struct{ Status int }

func (e YandexStatusError) Error() string { return fmt.Sprintf("cloud provider response %d", e.Status) }
func (e YandexStatusError) Is(target error) bool {
	switch target {
	case ErrYandexReauth:
		return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
	case ErrYandexResourceMissing:
		return e.Status == http.StatusNotFound
	case ErrYandexRateLimited:
		return e.Status == http.StatusTooManyRequests
	case ErrYandexProviderDown:
		return e.Status >= 500
	}
	return false
}

const yandexMaxAttempts = 3

var yandexRetryBaseDelay = 300 * time.Millisecond

func yandexRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
func yandexRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 0
	}
	if seconds > 3 {
		seconds = 3
	}
	return time.Duration(seconds) * time.Second
}
func YandexProviderError(err error) bool {
	return errors.Is(err, ErrYandexReauth) || errors.Is(err, ErrYandexResourceMissing) ||
		errors.Is(err, ErrYandexRateLimited) || errors.Is(err, ErrYandexProviderDown) ||
		errors.Is(err, ErrYandexTransport) || errors.Is(err, context.DeadlineExceeded)
}
