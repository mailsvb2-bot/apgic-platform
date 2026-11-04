package academy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)
type yandexScriptedTransport struct { statuses []int; calls int }
func (s *yandexScriptedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	status := s.statuses[len(s.statuses)-1]
	if s.calls < len(s.statuses) { status = s.statuses[s.calls] }
	s.calls++
	return &http.Response{StatusCode:status, Header:http.Header{"Retry-After":{"0"}}, Body:io.NopCloser(strings.NewReader("{}"))}, nil
}
func TestYandexDoRetriesTransientAndTypesTerminalErrors(t *testing.T) {
	old := yandexRetryBaseDelay
	yandexRetryBaseDelay = time.Millisecond
	defer func(){ yandexRetryBaseDelay = old }()
	cases := []struct { statuses []int; calls int; want error }{
		{[]int{429,503,200},3,nil},
		{[]int{429},3,ErrYandexRateLimited},
		{[]int{503},3,ErrYandexProviderDown},
		{[]int{401},1,ErrYandexReauth},
		{[]int{404},1,ErrYandexResourceMissing},
	}
	for _, tc := range cases {
		transport := &yandexScriptedTransport{statuses:tc.statuses}
		client := YandexDiskClient{Client:&http.Client{Transport:transport}}
		var body map[string]any
		err := client.do(context.Background(),"test","https://cloud-api.yandex.net/test",&body)
		if transport.calls != tc.calls || (tc.want == nil && err != nil) || (tc.want != nil && !errors.Is(err,tc.want)) {
			t.Fatalf("statuses=%v calls=%d error=%v",tc.statuses,transport.calls,err)
		}
	}
}
func TestYandexRetryAfterIsBounded(t *testing.T) {
	if yandexRetryAfter("120") != 3*time.Second || yandexRetryAfter("x") != 0 || yandexRetryAfter("1") != time.Second { t.Fatal("Retry-After must be capped") }
}
