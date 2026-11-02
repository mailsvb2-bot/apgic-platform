package runtimepostgres

import (
	"context"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyImportVerifiedCloudMedia is the sole HTTP import path. Verification
// and catalog persistence share a single OAuth connection generation. An
// OAuth reconnect between them fails closed rather than rebinding a disk:/
// path to a different author's file. No provider credential or URL is returned.
func (c *Checker) AcademyImportVerifiedCloudMedia(ctx context.Context, actor string, items []academy.CloudMedia) ([]academy.CloudMedia, error) {
	if !academyMediaUUID(actor) || len(items) == 0 || len(items) > 100 {
		return nil, academy.ErrInvalid
	}
	// Validate the ENTIRE batch before reading a credential or calling Yandex.
	for _, item := range items {
		if item.Provider != "YANDEX_DISK" || item.SourceURL != "" ||
			!academy.ValidYandexDiskPath(item.ProviderFileRef) ||
			len(item.Title) == 0 || len(item.Title) > 300 {
			return nil, academy.ErrInvalid
		}
	}
	token, generation, err := c.academyCloudCredentialSnapshot(actor, "YANDEX_DISK")
	if err != nil {
		return nil, academy.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	provider := academy.YandexDiskClient{}
	err = academyVerifyYandexMediaBatch(ctx, items, func(ctx context.Context, path string) error {
		if _, err := provider.VerifyVideo(ctx, token, path); err != nil {
			return err
		}
		_, err := provider.TemporaryDownload(ctx, token, path)
		return err
	})
	if err != nil || ctx.Err() != nil {
		return nil, academy.ErrDenied
	}
	// A transaction-level FOR SHARE of the connection generation below
	// prevents OAuth writes from racing the final catalog commit.
	return c.academyImportCloudMediaWithGeneration(actor, items, generation)
}
