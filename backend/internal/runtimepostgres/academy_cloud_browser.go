package runtimepostgres

import (
	"context"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyBrowseYandexDisk loads files exclusively with the authenticated
// author's encrypted credential. No OAuth token is returned to the browser.
func (c *Checker) AcademyBrowseYandexDisk(actor, path string, offset, limit int) ([]academy.YandexDiskFile, int, error) {
	token, err := c.AcademyReadCloudCredential(actor, "YANDEX_DISK")
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return (academy.YandexDiskClient{}).ListFiles(ctx, token, path, offset, limit)
}
