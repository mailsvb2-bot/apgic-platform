package runtimepostgres

import (
	"context"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyVerifyYandexDiskMedia is the provider-owned preflight for the
// authenticated HTTP import path. It never trusts browser-provided MIME,
// file existence or a cloud URL. The DB import remains idempotent and runs
// only after every selected item has been verified.
func (c *Checker) AcademyVerifyYandexDiskMedia(ctx context.Context, actor string, items []academy.CloudMedia) error {
	if !academyMediaUUID(actor) || len(items) == 0 || len(items) > 100 {
		return academy.ErrInvalid
	}
	token, err := c.AcademyReadCloudCredential(actor, "YANDEX_DISK")
	if err != nil {
		return academy.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	provider := academy.YandexDiskClient{}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Provider != "YANDEX_DISK" || item.SourceURL != "" ||
			item.ProviderFileRef == "" || len(item.ProviderFileRef) > 2048 {
			return academy.ErrInvalid
		}
		if seen[item.ProviderFileRef] {
			continue
		}
		seen[item.ProviderFileRef] = true
		if _, err := provider.VerifyVideo(ctx, token, item.ProviderFileRef); err != nil {
			// Do not return upstream errors, which can contain cloud paths,
			// request details or authorization information.
			return academy.ErrDenied
		}
	}
	return nil
}
