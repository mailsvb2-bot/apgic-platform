package httpapi

import (
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
	for _, tc := range []struct {
		name, query string
		allowed     bool
	}{
		{"no editor", "", true},
		{"valid editor", "course_id=" + course + "&version_id=" + version, true},
		{"course alone", "course_id=" + course, false},
		{"version alone", "version_id=" + version, false},
		{"duplicate course", "course_id=" + course + "&course_id=" + course + "&version_id=" + version, false},
		{"unknown redirect", "course_id=" + course + "&version_id=" + version + "&return_to=https%3A%2F%2Fevil.invalid", false},
		{"invalid course", "course_id=../../evil&version_id=" + version, false},
		{"malformed encoding", "course_id=%ZZ&version_id=" + version, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotCourse, gotVersion, allowed := cloudOAuthEditorReturnIDs(tc.query)
			if allowed != tc.allowed {
				t.Fatalf("accepted=%v, want %v", allowed, tc.allowed)
			}
			if allowed && tc.query != "" && (gotCourse != course || gotVersion != version) {
				t.Fatalf("wrong return target: %q %q", gotCourse, gotVersion)
			}
		})
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	state := cloudOAuthState{Actor: "author", Verifier: strings.Repeat("v", 43), Nonce: "nonce",
		Expires: time.Now().Add(time.Minute).Unix(), ReturnCourse: course, ReturnVersion: version}
	sealed, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := cloudOpen(sealed, key)
	if err != nil || opened.ReturnCourse != course || opened.ReturnVersion != version {
		t.Fatalf("editor target not preserved by signed OAuth state: %+v %v", opened, err)
	}
	want := "/academy?cloud_connected=yandex&course_id=" + course + "&version_id=" + version
	if got := cloudOAuthCallbackDestination(opened); got != want {
		t.Fatalf("unexpected callback destination %q", got)
	}
	if got := cloudOAuthCallbackDestination(cloudOAuthState{}); got != "/academy?cloud_connected=yandex" {
		t.Fatalf("unexpected default callback destination %q", got)
	}
	state.ReturnVersion = ""
	invalid, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloudOpen(invalid, key); err == nil {
		t.Fatal("signed incomplete editor target was accepted")
	}
}
