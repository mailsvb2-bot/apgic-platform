package runtimepostgres

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// cloudBrowseFailureCategory deliberately emits only fixed labels. Never log
// OAuth tokens, provider URLs, author IDs, file paths, or raw upstream errors.
func cloudBrowseFailureCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, academy.ErrYandexTransport) {
		return "provider_transport"
	}
	if errors.Is(err, academy.ErrYandexJSON) {
		return "provider_json"
	}
	if errors.Is(err, academy.ErrYandexListing) {
		switch err.Error() {
		case "cloud provider listing invalid: pagination":
			return "provider_listing_pagination"
		case "cloud provider listing invalid: entry":
			return "provider_listing_entry"
		case "cloud provider listing invalid: duplicate":
			return "provider_listing_duplicate"
		}
		return "provider_listing"
	}
	if errors.Is(err, academy.ErrDenied) {
		return "credential_unavailable"
	}
	if errors.Is(err, academy.ErrUnavailable) {
		return "credential_unreadable"
	}
	if err != nil {
		for _, code := range []string{"401", "403", "404", "429", "500", "502", "503"} {
			if err.Error() == "cloud provider response "+code {
				return "provider_http_" + code
			}
		}
		if strings.HasPrefix(err.Error(), "cloud provider response ") {
			return "provider_http_other"
		}
	}
	return "provider_network_or_payload"
}

// AcademyBrowseYandexDisk loads files exclusively with the authenticated
// author's encrypted credential. No OAuth token is returned to the browser.
func (c *Checker) AcademyBrowseYandexDisk(actor, path string, offset, limit int) ([]academy.YandexDiskFile, int, error) {
	token, err := c.AcademyReadCloudCredential(actor, "YANDEX_DISK")
	if err != nil {
		log.Printf("academy_cloud_browse_failed stage=credential category=%s", cloudBrowseFailureCategory(err))
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	files, total, err := (academy.YandexDiskClient{}).ListFiles(ctx, token, path, offset, limit)
	if err != nil {
		log.Printf("academy_cloud_browse_failed stage=provider category=%s", cloudBrowseFailureCategory(err))
	}
	return files, total, err
}
