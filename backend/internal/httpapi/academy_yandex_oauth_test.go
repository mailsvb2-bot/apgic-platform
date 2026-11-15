package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCloudOAuthSignedStateRejectsTamperingAndExpiry(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	state := cloudOAuthState{
		Actor:    "author-one",
		Verifier: strings.Repeat("v", 43),
		Nonce:    "random-nonce",
		Expires:  time.Now().Add(time.Minute).Unix(),
	}
	sealed, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := cloudOpen(sealed, key)
	if err != nil || opened.Actor != state.Actor || opened.Nonce != state.Nonce {
		t.Fatalf("state round trip: %+v, %v", opened, err)
	}
	if _, err := cloudOpen(sealed, []byte("different-secret-key-012345678901")); err == nil {
		t.Fatal("foreign key accepted")
	}
	parts := strings.Split(sealed, ".")
	if len(parts) != 2 {
		t.Fatal("missing state signature")
	}
	parts[1] = strings.Repeat("A", len(parts[1]))
	if _, err := cloudOpen(strings.Join(parts, "."), key); err == nil {
		t.Fatal("tampered signature accepted")
	}
	state.Expires = time.Now().Add(-time.Minute).Unix()
	expired, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloudOpen(expired, key); err == nil {
		t.Fatal("expired OAuth flow accepted")
	}
}

func TestCloudOAuthCallbackRejectsSessionSwitch(t *testing.T) {
	state := cloudOAuthState{Actor: "author-one"}
	for _, tc := range []struct {
		name  string
		actor string
		allow bool
	}{
		{name: "same signed author", actor: "author-one", allow: true},
		{name: "another signed author", actor: "author-two", allow: false},
		{name: "logged out", actor: "", allow: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cloudOAuthActorMatches(state, tc.actor); got != tc.allow {
				t.Fatalf("callback actor match=%v, want %v", got, tc.allow)
			}
		})
	}
	if cloudOAuthActorMatches(cloudOAuthState{}, "author-one") {
		t.Fatal("empty state actor accepted")
	}
}

func TestCloudOAuthEditorReturnRequiresExactPair(t *testing.T) {
	const course = "11111111-1111-4111-8111-111111111111"
	const version = "22222222-2222-4222-8222-222222222222"
	const lesson = "33333333-3333-4333-8333-333333333333"
	for _, tc := range []struct {
		name, query string
		allowed     bool
	}{
		{"no editor", "", true},
		{"valid editor", "course_id=" + course + "&version_id=" + version, true},
		{"valid editor lesson", "course_id=" + course + "&version_id=" + version + "&lesson_id=" + lesson, true},
		{"invalid lesson", "course_id=" + course + "&version_id=" + version + "&lesson_id=../../evil", false},
		{"course alone", "course_id=" + course, false},
		{"version alone", "version_id=" + version, false},
		{"duplicate course", "course_id=" + course + "&course_id=" + course + "&version_id=" + version, false},
		{"unknown redirect", "course_id=" + course + "&version_id=" + version + "&return_to=https%3A%2F%2Fevil.invalid", false},
		{"invalid course", "course_id=../../evil&version_id=" + version, false},
		{"malformed encoding", "course_id=%ZZ&version_id=" + version, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotCourse, gotVersion, gotLesson, allowed := cloudOAuthEditorReturnIDs(tc.query)
			if allowed != tc.allowed {
				t.Fatalf("accepted=%v, want %v", allowed, tc.allowed)
			}
			if allowed && tc.query != "" && (gotCourse != course || gotVersion != version) {
				t.Fatalf("wrong return target: %q %q", gotCourse, gotVersion)
			}
			if tc.name == "valid editor lesson" && gotLesson != lesson {
				t.Fatalf("wrong return lesson: %q", gotLesson)
			}
		})
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	state := cloudOAuthState{Actor: "author", Verifier: strings.Repeat("v", 43), Nonce: "nonce",
		Expires: time.Now().Add(time.Minute).Unix(), ReturnCourse: course, ReturnVersion: version, ReturnLesson: lesson}
	sealed, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := cloudOpen(sealed, key)
	if err != nil || opened.ReturnCourse != course || opened.ReturnVersion != version || opened.ReturnLesson != lesson {
		t.Fatalf("editor target not preserved by signed OAuth state: %+v %v", opened, err)
	}
	want := "/academy?cloud_connected=yandex&course_id=" + course + "&lesson_id=" + lesson + "&version_id=" + version
	if got := cloudOAuthCallbackDestination(opened); got != want {
		t.Fatalf("unexpected callback destination %q", got)
	}
	if got := cloudOAuthCallbackDestination(cloudOAuthState{}); got != "/academy?cloud_connected=yandex" {
		t.Fatalf("unexpected default callback destination %q", got)
	}
	state.ReturnLesson = ""
	state.ReturnVersion = ""
	invalid, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloudOpen(invalid, key); err == nil {
		t.Fatal("signed incomplete editor target was accepted")
	}
}

func TestAcademyExchangeYandexOAuthCodeSendsPKCEAndClientAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%q, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("content type=%q", got)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-client" || password != "test-secret" {
			t.Errorf("client authentication was not sent with HTTP Basic")
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token request: %v", err)
		}
		for key, want := range map[string]string{
			"grant_type":    "authorization_code",
			"code":          "one-time-code",
			"code_verifier": strings.Repeat("v", 43),
		} {
			if got := r.Form.Get(key); got != want {
				t.Errorf("%s=%q, want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"short-lived-test-token","token_type":"Bearer"}`))
	}))
	defer server.Close()

	token, err := academyExchangeYandexOAuthCode(
		context.Background(), server.Client(), server.URL,
		"test-client", "test-secret", "one-time-code", strings.Repeat("v", 43),
	)
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if token != "short-lived-test-token" {
		t.Fatalf("unexpected token result %q", token)
	}
}

func TestAcademyExchangeYandexOAuthCodeNeverFollowsRedirectWithClientSecret(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := academyExchangeYandexOAuthCode(
		context.Background(), redirect.Client(), redirect.URL,
		"test-client", "test-secret", "one-time-code", strings.Repeat("v", 43),
	)
	if err == nil {
		t.Fatal("redirect response was accepted as a successful token exchange")
	}
	if targetHits != 0 {
		t.Fatalf("redirect target received %d requests carrying the OAuth exchange", targetHits)
	}
}

func TestAcademyExchangeYandexOAuthCodeRejectsMalformedTokenResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "trailing JSON", body: `{"access_token":"token","token_type":"Bearer"}{}`},
		{name: "unsupported token type", body: `{"access_token":"token","token_type":"Basic"}`},
		{name: "empty token", body: `{"access_token":"","token_type":"Bearer"}`},
		{name: "header injection", body: `{"access_token":"token\r\nInjected: yes","token_type":"Bearer"}`},
		{name: "oversized response", body: strings.Repeat("x", 16385)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			if token, err := academyExchangeYandexOAuthCode(
				context.Background(), server.Client(), server.URL,
				"test-client", "test-secret", "one-time-code", strings.Repeat("v", 43),
			); err == nil || token != "" {
				t.Fatalf("malformed token response accepted: token=%q err=%v", token, err)
			}
		})
	}
}

func TestCloudOAuthCallbackCodeRejectsAmbiguousQuery(t *testing.T) {
	for _, raw := range []string{
		"state=nonce&state=other&code=one-time-code",
		"state=nonce&code=one-time-code&extra=value",
		"state=nonce&code=one-time-code&cid=bad.value",
		"state=nonce&code=one-time-code&cid=ok&cid=duplicate",
		"state=nonce&code=%0d%0aInjected",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, ok := cloudOAuthCallbackCodeFromRaw(raw, "nonce"); ok {
				t.Fatalf("ambiguous OAuth callback query accepted: %q", raw)
			}
		})
	}
	if code, ok := cloudOAuthCallbackCodeFromRaw(url.Values{
		"state": {"nonce"}, "code": {"one-time-code"}, "cid": {"safe_123"},
	}.Encode(), "nonce"); !ok || code != "one-time-code" {
		t.Fatalf("valid Yandex callback rejected: code=%q accepted=%v", code, ok)
	}
}


func TestAcademyYandexOAuthConnectSetsS256PKCEAndSignedEditorState(t *testing.T) {
	const authorID = "11111111-1111-4111-8111-111111111111"
	const courseID = "22222222-2222-4222-8222-222222222222"
	const versionID = "33333333-3333-4333-8333-333333333333"
	const lessonID = "44444444-4444-4444-8444-444444444444"
	sessionKey := []byte(strings.Repeat("session-key-", 4))
	stateKey := []byte(strings.Repeat("oauth-state-key-", 3))
	t.Setenv("APGIC_YANDEX_OAUTH_CLIENT_ID", "test-yandex-client")
	t.Setenv("APGIC_YANDEX_OAUTH_CLIENT_SECRET", "test-yandex-secret")
	t.Setenv("APGIC_YANDEX_OAUTH_REDIRECT_URI", "https://api.apgic.example/v1/academy/author/cloud/yandex/callback")
	t.Setenv("APGIC_ACADEMY_OAUTH_STATE_KEY", base64.StdEncoding.EncodeToString(stateKey))

	sessions, err := newClientSessionManager(sessionKey, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authorCookie, err := sessions.issue(authorID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{ClientSessionKey: sessionKey})
	request := httptest.NewRequest(http.MethodGet,
		"https://apgic.example/v1/academy/author/cloud/yandex/connect?course_id="+courseID+"&version_id="+versionID+"&lesson_id="+lessonID,
		nil)
	request.AddCookie(authorCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("connect status=%d want=%d body=%s", response.Code, http.StatusFound, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil || location.Scheme != "https" || location.Host != "oauth.yandex.ru" || location.Path != "/authorize" {
		t.Fatalf("unexpected OAuth authorization destination: %q (%v)", response.Header().Get("Location"), err)
	}
	query := location.Query()
	if query.Get("response_type") != "code" || query.Get("client_id") != "test-yandex-client" ||
		query.Get("redirect_uri") != "https://api.apgic.example/v1/academy/author/cloud/yandex/callback" ||
		query.Get("code_challenge_method") != "S256" {
		t.Fatalf("incomplete OAuth authorization request: %v", query)
	}
	for key, values := range query {
		if len(values) != 1 {
			t.Fatalf("duplicate OAuth authorization parameter %q: %v", key, values)
		}
		switch key {
		case "response_type", "client_id", "redirect_uri", "state", "code_challenge", "code_challenge_method":
		default:
			t.Fatalf("unexpected OAuth authorization parameter %q", key)
		}
	}
	cookies := response.Result().Cookies()
	var stateCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "apgic_academy_yandex_oauth" {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("signed OAuth state cookie was not issued")
	}
	if !stateCookie.Secure || !stateCookie.HttpOnly || stateCookie.SameSite != http.SameSiteLaxMode ||
		stateCookie.Path != "/v1/academy/author/cloud/yandex" || stateCookie.MaxAge != 600 {
		t.Fatalf("OAuth state cookie is not hardened: %#v", stateCookie)
	}
	state, err := cloudOpen(stateCookie.Value, stateKey)
	if err != nil {
		t.Fatalf("signed OAuth state could not be opened: %v", err)
	}
	if state.Actor != authorID || state.ReturnCourse != courseID || state.ReturnVersion != versionID ||
		state.ReturnLesson != lessonID || state.Nonce == "" || query.Get("state") != state.Nonce {
		t.Fatalf("OAuth state lost signed author/editor binding: %+v", state)
	}
	challenge := sha256.Sum256([]byte(state.Verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(challenge[:])
	if state.Verifier == "" || query.Get("code_challenge") != wantChallenge {
		t.Fatal("authorization request does not use the S256 challenge for its signed PKCE verifier")
	}
	if strings.Contains(response.Header().Get("Location"), state.Verifier) ||
		strings.Contains(response.Header().Get("Location"), stateCookie.Value) {
		t.Fatal("PKCE verifier or signed state cookie leaked into the authorization URL")
	}
}
