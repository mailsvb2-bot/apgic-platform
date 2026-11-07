package runtimepostgres

import (
	"context"
	"sync"
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
	verified := make(map[string]academy.YandexDiskFile, len(items))
	var mu sync.Mutex
	err = academyVerifyYandexMediaBatch(ctx, items, func(ctx context.Context, path string) error {
		file, err := provider.VerifyVideo(ctx, token, path)
		if err != nil {
			return err
		}
		if _, err := provider.TemporaryDownload(ctx, token, path); err != nil {
			return err
		}
		mu.Lock()
		verified[path] = file
		mu.Unlock()
		return nil
	})
	if err != nil && academy.YandexProviderError(err) {
		return nil, err
	}
	if err != nil || ctx.Err() != nil {
		return nil, academy.ErrDenied
	}
	bound := make([]academy.CloudMedia, len(items))
	copy(bound, items)
	for i := range bound {
		file, ok := verified[bound[i].ProviderFileRef]
		if !ok {
			return nil, academy.ErrDenied
		}
		bound[i].ResourceID = file.ResourceID
		bound[i].SHA256 = file.SHA256
		bound[i].FileSize = file.Size
	}
	// A transaction-level FOR SHARE of the connection generation below
	// prevents OAuth writes from racing the final catalog commit.
	return c.academyImportCloudMediaWithGeneration(actor, bound, generation)
}
