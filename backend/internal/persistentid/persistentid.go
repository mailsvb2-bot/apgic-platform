package persistentid

import (
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRef = errors.New("persistent id reference is required")

var namespace = [16]byte{
	0xa5, 0x70, 0x67, 0x69,
	0x63, 0x2d,
	0x50, 0x52,
	0x91, 0x10,
	0x42, 0x4f, 0x4f, 0x4b, 0x49, 0x4e,
}

func New() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return format(raw), nil
}

func FromRef(scope, ref string) (string, error) {
	scope = strings.TrimSpace(scope)
	ref = strings.TrimSpace(ref)
	if scope == "" || ref == "" {
		return "", ErrInvalidRef
	}
	hash := sha1.New()
	_, _ = hash.Write(namespace[:])
	_, _ = hash.Write([]byte(scope))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(ref))
	sum := hash.Sum(nil)
	var raw [16]byte
	copy(raw[:], sum[:16])
	raw[6] = (raw[6] & 0x0f) | 0x50
	raw[8] = (raw[8] & 0x3f) | 0x80
	return format(raw), nil
}

func format(raw [16]byte) string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	)
}
