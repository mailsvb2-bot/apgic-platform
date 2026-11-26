package httpapi

import (
	"net/http"
	"testing"
)

func TestAcademyVideoRequestedRangeRejectsNoncanonicalDuplicates(t *testing.T) {
	cases := []struct {
		name    string
		headers http.Header
		want    string
		ok      bool
	}{
		{name: "absent", headers: http.Header{}, ok: true},
		{name: "canonical", headers: http.Header{"Range": {"bytes=1-3"}}, want: "bytes=1-3", ok: true},
		{name: "lowercase", headers: http.Header{"range": {"bytes=1-3"}}, want: "bytes=1-3", ok: true},
		{name: "uppercase", headers: http.Header{"RANGE": {"bytes=1-3"}}, want: "bytes=1-3", ok: true},
		{name: "different casing duplicate", headers: http.Header{"Range": {"bytes=1-3"}, "range": {"bytes=4-6"}}},
		{name: "different casing duplicate identical", headers: http.Header{"Range": {"bytes=1-3"}, "range": {"bytes=1-3"}}},
		{name: "two fields", headers: http.Header{"Range": {"bytes=1-3", "bytes=4-6"}}},
		{name: "empty field slice", headers: http.Header{"range": {}}},
		{name: "invalid lower case range", headers: http.Header{"range": {"bytes=1-2,4-5"}}},
		{name: "empty range value", headers: http.Header{"Range": {""}}},
		{name: "whitespace range value", headers: http.Header{"range": {" bytes=1-3"}}},
		{name: "duplicate with empty slice", headers: http.Header{"Range": {"bytes=1-3"}, "range": {}}},
		{name: "duplicate with empty value", headers: http.Header{"Range": {"bytes=1-3"}, "range": {""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := academyVideoRequestedRange(tc.headers)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("range = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
