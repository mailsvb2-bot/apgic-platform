package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCloudImportVerifierStub struct {
	calls int
	err   error
}

func (s *academyCloudImportVerifierStub) AcademyVerifyYandexDiskMedia(_ context.Context, _ string, _ []academy.CloudMedia) error {
	s.calls++
	return s.err
}

func TestAcademyCloudImportRequiresVerifiedPrivateVideo(t *testing.T) {
	ctx := context.Background()
	video := academy.CloudMedia{
		Provider: "YANDEX_DISK", ProviderFileRef: "disk:/lectures/one.mp4", Title: "Lesson",
	}
	tests := []struct {
		name  string
		items []academy.CloudMedia
		ok    bool
		calls int
	}{
		{"verified Yandex file", []academy.CloudMedia{video}, true, 1},
		{"empty batch", nil, false, 0},
		{"external public source", []academy.CloudMedia{{
			Provider: "EXTERNAL", ProviderFileRef: "public.mp4", SourceURL: "https://example.org/video.mp4",
		}}, false, 0},
		{"Yandex with source URL", []academy.CloudMedia{{
			Provider: "YANDEX_DISK", ProviderFileRef: "disk:/lectures/one.mp4", SourceURL: "https://example.org/video.mp4",
		}}, false, 0},
		{"oversized batch", make([]academy.CloudMedia, 101), false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &academyCloudImportVerifierStub{}
			err := academyVerifyCloudImport(ctx, "signed-author", tt.items, stub)
			if (err == nil) != tt.ok {
				t.Fatalf("unexpected verification result: %v", err)
			}
			if stub.calls != tt.calls {
				t.Fatalf("provider verification calls=%d, expected %d", stub.calls, tt.calls)
			}
		})
	}
	if err := academyVerifyCloudImport(ctx, "signed-author", []academy.CloudMedia{video}, nil); !errors.Is(err, academy.ErrUnavailable) {
		t.Fatalf("missing provider verifier must fail closed: %v", err)
	}
	stub := &academyCloudImportVerifierStub{err: academy.ErrDenied}
	if err := academyVerifyCloudImport(ctx, "signed-author", []academy.CloudMedia{video}, stub); !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("provider denial was not enforced: %v", err)
	}
}
