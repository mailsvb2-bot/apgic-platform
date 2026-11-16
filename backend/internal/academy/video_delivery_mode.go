package academy

import (
	"errors"
	"os"
	"strings"
)

type VideoDeliveryMode string

const (
	VideoDeliveryDirect VideoDeliveryMode = "direct"
	VideoDeliveryCDN    VideoDeliveryMode = "cdn"
)

var ErrVideoDeliveryMode = errors.New("unsupported Academy video delivery mode")

// ConfiguredVideoDeliveryMode selects only the transport. Academy remains
// the sole owner of enrollment, payment, media mapping and lesson progress.
// The default is the first-party Yandex gateway. Unknown values fail closed.
// CDN can be selected only after its runtime adapter and independent provider
// acceptance are connected; switching modes never manufactures approval.
func ConfiguredVideoDeliveryMode() (VideoDeliveryMode, error) {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("APGIC_ACADEMY_VIDEO_DELIVERY_MODE"))) {
	case "", string(VideoDeliveryDirect):
		return VideoDeliveryDirect, nil
	case string(VideoDeliveryCDN):
		return VideoDeliveryCDN, nil
	default:
		return "", ErrVideoDeliveryMode
	}
}
