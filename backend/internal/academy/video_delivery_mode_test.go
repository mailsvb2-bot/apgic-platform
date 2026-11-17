package academy

import (
	"errors"
	"testing"
)

func TestConfiguredVideoDeliveryMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  VideoDeliveryMode
		err   bool
	}{
		{name: "default", value: "", want: VideoDeliveryDirect},
		{name: "explicit direct", value: "direct", want: VideoDeliveryDirect},
		{name: "cdn", value: "cdn", want: VideoDeliveryCDN},
		{name: "case and whitespace", value: " CDN ", want: VideoDeliveryCDN},
		{name: "unknown fail closed", value: "other", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APGIC_ACADEMY_VIDEO_DELIVERY_MODE", tc.value)
			got, err := ConfiguredVideoDeliveryMode()
			if tc.err {
				if !errors.Is(err, ErrVideoDeliveryMode) || got != "" {
					t.Fatalf("invalid mode accepted: %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}
