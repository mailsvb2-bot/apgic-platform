package academy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	Embedded struct {
		Items []YandexDiskFile `json:"items"`
		Total int              `json:"total"`
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
	ErrYandexTransport = errors.New("cloud transport failed")
	ErrYandexJSON      = errors.New("cloud provider JSON invalid")
	ErrYandexListing   = errors.New("cloud provider listing invalid")
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
	resp, err := protected.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return ErrYandexTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloud provider response %d", resp.StatusCode)
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
func (c YandexDiskClient) ListFiles(ctx context.Context, token, path string, offset, limit int) ([]YandexDiskFile, int, error) {
	if !ValidYandexDiskPath(path) || offset < 0 || offset > 1000000 || limit < 1 || limit > 1000 {
		return nil, 0, errors.New("invalid cloud file request")
	}
	q := url.Values{"path": {path}, "offset": {fmt.Sprint(offset)}, "limit": {fmt.Sprint(limit)}, "fields": {"_embedded.items.name,_embedded.items.path,_embedded.items.type,_embedded.items.mime_type,_embedded.items.size,_embedded.total"}}
	var d yandexDiskFilesResponse
	if err := c.do(ctx, token, yandexDiskAPI+"?"+q.Encode(), &d); err != nil {
		return nil, 0, err
	}
	// Provider metadata is not an authorization decision. Reject malformed
	// folder entries before they become browser navigation/import candidates.
	// Only direct children of the requested folder may be displayed.
	if d.Embedded.Total < 0 || len(d.Embedded.Items) > limit ||
		(len(d.Embedded.Items) > 0 && (offset > d.Embedded.Total || len(d.Embedded.Items) > d.Embedded.Total-offset)) {
		return nil, 0, fmt.Errorf("%w: pagination", ErrYandexListing)
	}
	prefix := strings.TrimSuffix(path, "/") + "/"
	seen := make(map[string]struct{}, len(d.Embedded.Items))
	for _, file := range d.Embedded.Items {
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
	return d.Embedded.Items, d.Embedded.Total, nil
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
