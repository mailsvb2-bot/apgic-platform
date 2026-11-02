package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudOAuthCallbackRejectsAmbiguousParameters(t *testing.T) {
	cases := []struct {
		name  string
		query url.Values
		ok    bool
	}{
		{"single valid pair", url.Values{"state": {"nonce"}, "code": {"one-time-code"}}, true},
		{"duplicate state", url.Values{"state": {"nonce", "nonce"}, "code": {"code"}}, false},
		{"duplicate code", url.Values{"state": {"nonce"}, "code": {"first", "second"}}, false},
		{"missing state", url.Values{"code": {"code"}}, false},
		{"missing code", url.Values{"state": {"nonce"}}, false},
		{"provider error", url.Values{"state": {"nonce"}, "code": {"code"}, "error": {"access_denied"}}, false},
		{"unknown callback parameter", url.Values{"state": {"nonce"}, "code": {"code"}, "redirect_uri": {"https://evil.invalid"}}, false},
		{"empty extra callback parameter", url.Values{"state": {"nonce"}, "code": {"code"}, "extra": {""}}, false},
		{"provider error description", url.Values{"state": {"nonce"}, "code": {"code"}, "error_description": {"denied"}}, false},
		{"wrong state", url.Values{"state": {"foreign"}, "code": {"code"}}, false},
		{"blank code", url.Values{"state": {"nonce"}, "code": {""}}, false},
		{"newline code", url.Values{"state": {"nonce"}, "code": {"code\nheader"}}, false},
		{"oversized code", url.Values{"state": {"nonce"}, "code": {strings.Repeat("x", 2049)}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := cloudOAuthCallbackCode(tc.query, "nonce")
			if ok != tc.ok || (ok && code != tc.query.Get("code")) {
				t.Fatalf("callback code=%q allowed=%v, want %v", code, ok, tc.ok)
			}
		})
	}
}

func TestAcademyYandexOAuthCodeExchangeTransport(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantToken bool
	}{
		{"valid bearer", 200, `{"access_token":"private-token","token_type":"bearer"}`, true},
		{"valid legacy without token type", 200, `{"access_token":"private-token"}`, true},
		{"provider denied", 401, `{"error":"invalid_grant"}`, false},
		{"redirect forbidden", 302, "", false},
		{"trailing JSON rejected", 200, `{"access_token":"private-token"}{"extra":true}`, false},
		{"wrong token type", 200, `{"access_token":"private-token","token_type":"mac"}`, false},
		{"missing token", 200, `{"token_type":"bearer"}`, false},
		{"oversized body", 200, `{"access_token":"private-token","extra":"` + strings.Repeat("x", 17000) + `"}`, false},
		{"newline token", 200, `{"access_token":"bad\ntoken"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("invalid OAuth token request")
				}
				id, secret, ok := r.BasicAuth()
				if !ok || id != "client-id" || secret != "client-secret" {
					t.Errorf("missing client authentication")
				}
				if err := r.ParseForm(); err != nil || r.PostForm.Get("code") != "auth-code" ||
					r.PostForm.Get("code_verifier") != "pkce-verifier" ||
					r.PostForm.Get("grant_type") != "authorization_code" {
					t.Errorf("missing OAuth code/PKCE binding: %v", err)
				}
				if tc.status == http.StatusFound {
					w.Header().Set("Location", "https://example.invalid/steal")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := &http.Client{Timeout: 3 * time.Second}
			token, err := academyExchangeYandexOAuthCode(context.Background(), client, server.URL,
				"client-id", "client-secret", "auth-code", "pkce-verifier")
			if tc.wantToken {
				if err != nil || token != "private-token" {
					t.Fatalf("token exchange: token=%q err=%v", token, err)
				}
			} else if err == nil || token != "" {
				t.Fatalf("unsafe provider response accepted: token=%q err=%v", token, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("token endpoint called %d times", calls.Load())
			}
		})
	}
}

func TestCloudOAuthCallbackRejectsMalformedRawQuery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   string
		valid bool
	}{
		{"valid", "state=nonce&code=one-time-code", true},
		{"malformed escape after valid code", "state=nonce&code=one-time-code&%ZZ=ignored", false},
		{"malformed escape in code", "state=nonce&code=bad%ZZ", false},
		{"ambiguous duplicate", "state=nonce&code=first&code=second", false},
		{"unexpected query key", "state=nonce&code=one-time-code&next=%2Fadmin", false},
		{"invalid semicolon separator", "state=nonce&code=one-time-code&extra=1;other=2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := cloudOAuthCallbackCodeFromRaw(tc.raw, "nonce")
			if ok != tc.valid || (ok && code != "one-time-code") {
				t.Fatalf("raw callback accepted=%v code=%q, want valid=%v", ok, code, tc.valid)
			}
		})
	}
}
