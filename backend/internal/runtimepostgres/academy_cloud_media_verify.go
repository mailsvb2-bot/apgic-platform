package runtimepostgres

import (
	"context"
	"sync"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyVerifyYandexDiskMedia checks the entire bounded batch before the
// catalog transaction. Parallel provider preflights keep large author imports
// within the request deadline without exposing credentials or download URLs.
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
	return academyVerifyYandexMediaBatch(ctx, items, func(ctx context.Context, path string) error {
		if _, err := provider.VerifyVideo(ctx, token, path); err != nil {
			return err
		}
		_, err := provider.TemporaryDownload(ctx, token, path)
		return err
	})
}

// Validate the complete batch before contacting the provider. At most four
// files are verified simultaneously; each file's metadata and private download
// preflight are performed in order with the same cancellable context.
func academyVerifyYandexMediaBatch(ctx context.Context, items []academy.CloudMedia, verify func(context.Context, string) error) error {
	if len(items) == 0 || len(items) > 100 || verify == nil {
		return academy.ErrInvalid
	}
	seen := make(map[string]struct{}, len(items))
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.Provider != "YANDEX_DISK" || item.SourceURL != "" ||
			!academy.ValidYandexDiskPath(item.ProviderFileRef) {
			return academy.ErrInvalid
		}
		if _, ok := seen[item.ProviderFileRef]; !ok {
			seen[item.ProviderFileRef] = struct{}{}
			paths = append(paths, item.ProviderFileRef)
		}
	}
	if ctx.Err() != nil {
		return academy.ErrDenied
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	var wg sync.WaitGroup
	var once sync.Once
	var providerErr error
	workers := 4
	if len(paths) < workers {
		workers = len(paths)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case path, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					if err := verify(ctx, path); err != nil {
						once.Do(func() {
							providerErr = academy.ErrDenied
							if academy.YandexProviderError(err) { providerErr = err }
							cancel()
						})
						return
					}
				}
			}
		}()
	}
submit:
	for _, path := range paths {
		select {
		case <-ctx.Done():
			break submit
		case jobs <- path:
		}
	}
	close(jobs)
	wg.Wait()
	if providerErr != nil { return providerErr }
	if ctx.Err() != nil { return academy.ErrDenied }
	return nil
}
