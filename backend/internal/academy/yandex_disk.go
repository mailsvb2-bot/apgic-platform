package academy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
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
		client = &http.Client{Timeout: 15 * time.Second}
	}
	// A provider redirect must never receive the author's OAuth credential.
	// Clone the client to enforce this even when an injected client permits redirects.
	protected := *client
	protected.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := protected.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloud provider response %d", resp.StatusCode)
	}
	return json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 2<<20)).Decode(dst)
}

// ListFiles uses bounded pagination: a browser can batch-select an entire
// folder while the API never returns the secret token.
func (c YandexDiskClient) ListFiles(ctx context.Context, token, path string, offset, limit int) ([]YandexDiskFile, int, error) {
	if !strings.HasPrefix(path, "disk:/") || offset < 0 || limit < 1 || limit > 1000 {
		return nil, 0, errors.New("invalid cloud file request")
	}
	q := url.Values{"path": {path}, "offset": {fmt.Sprint(offset)}, "limit": {fmt.Sprint(limit)}, "fields": {"_embedded.items.name,_embedded.items.path,_embedded.items.type,_embedded.items.mime_type,_embedded.items.size,_embedded.total"}}
	var d yandexDiskFilesResponse
	if err := c.do(ctx, token, yandexDiskAPI+"?"+q.Encode(), &d); err != nil {
		return nil, 0, err
	}
	return d.Embedded.Items, d.Embedded.Total, nil
}

// TemporaryDownload is used ONLY server-side by the media gateway.
// The result must never be included in an author or learner API JSON response.
func (c YandexDiskClient) TemporaryDownload(ctx context.Context, token, path string) (string, error) {
	if !strings.HasPrefix(path, "disk:/") {
		return "", errors.New("invalid disk path")
	}
	q := url.Values{"path": {path}}
	var d yandexDiskDownloadResponse
	if err := c.do(ctx, token, yandexDiskAPI+"/download?"+q.Encode(), &d); err != nil {
		return "", err
	}
	u, err := url.Parse(d.Href)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || !trustedYandexDownloadHost(u.Hostname()) || d.Method != "GET" || d.Templated {
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
