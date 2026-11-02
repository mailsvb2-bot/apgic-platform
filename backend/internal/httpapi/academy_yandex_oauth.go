package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCloudCredentialWriter interface {
	AcademyStoreCloudCredential(actor, provider, token string) error
}
type cloudOAuthState struct {
	Actor         string `json:"actor"`
	Verifier      string `json:"verifier"`
	Nonce         string `json:"nonce"`
	ReturnCourse  string `json:"return_course,omitempty"`
	ReturnVersion string `json:"return_version,omitempty"`
	Expires       int64  `json:"expires"`
}

func cloudRandom(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func cloudStateKey() ([]byte, error) {
	s := os.Getenv("APGIC_ACADEMY_OAUTH_STATE_KEY")
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(key) < 32 {
		return nil, errors.New("OAuth state key is unavailable")
	}
	return key, nil
}
func cloudSeal(state cloudOAuthState, key []byte) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func cloudOpen(value string, key []byte) (cloudOAuthState, error) {
	var state cloudOAuthState
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return state, academy.ErrDenied
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(raw) > 2048 {
		return state, academy.ErrDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return state, academy.ErrDenied
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return state, academy.ErrDenied
	}
	if json.Unmarshal(raw, &state) != nil || state.Actor == "" || state.Verifier == "" || state.Nonce == "" || state.Expires < time.Now().Unix() ||
		(state.ReturnCourse == "") != (state.ReturnVersion == "") ||
		(state.ReturnCourse != "" && (!cloudOAuthEditorIDPattern.MatchString(state.ReturnCourse) || !cloudOAuthEditorIDPattern.MatchString(state.ReturnVersion))) {
		return state, academy.ErrDenied
	}
	return state, nil
}

// A callback may finish only under the same signed APGIC identity that
// initiated the OAuth flow. Switching accounts must never attach another
// person's cloud credentials to the original author.
func cloudOAuthActorMatches(state cloudOAuthState, actor string) bool {
	return actor != "" && state.Actor != "" && state.Actor == actor
}

// Only a pair of canonical course/version UUIDs may survive the OAuth
// round-trip. No browser-controlled return URL or open redirect is accepted.
var cloudOAuthEditorIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func cloudOAuthEditorReturnIDs(rawQuery string) (string, string, bool) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil || len(query) > 2 {
		return "", "", false
	}
	if len(query) == 0 {
		return "", "", true
	}
	course, version := query["course_id"], query["version_id"]
	if len(course) != 1 || len(version) != 1 ||
		!cloudOAuthEditorIDPattern.MatchString(course[0]) ||
		!cloudOAuthEditorIDPattern.MatchString(version[0]) {
		return "", "", false
	}
	return course[0], version[0], true
}

func cloudOAuthCallbackDestination(state cloudOAuthState) string {
	query := url.Values{"cloud_connected": {"yandex"}}
	if state.ReturnCourse != "" && state.ReturnVersion != "" {
		query.Set("course_id", state.ReturnCourse)
		query.Set("version_id", state.ReturnVersion)
	}
	return "/academy?" + query.Encode()
}

func academyYandexOAuthConfig() (string, string, string, error) {
	id := os.Getenv("APGIC_YANDEX_OAUTH_CLIENT_ID")
	secret := os.Getenv("APGIC_YANDEX_OAUTH_CLIENT_SECRET")
	redirect := os.Getenv("APGIC_YANDEX_OAUTH_REDIRECT_URI")
	u, err := url.Parse(redirect)
	if id == "" || secret == "" || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Path != "/v1/academy/author/cloud/yandex/callback" || u.RawQuery != "" {
		return "", "", "", academy.ErrUnavailable
	}
	return id, secret, redirect, nil
}

// cloudOAuthCallbackCodeFromRaw rejects malformed query escapes instead of
// silently ignoring them as url.URL.Query does. OAuth callbacks must have
// an unambiguous, fully parsed state/code pair.
func cloudOAuthCallbackCodeFromRaw(rawQuery, expectedState string) (string, bool) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false
	}
	return cloudOAuthCallbackCode(query, expectedState)
}

// cloudOAuthCallbackCode accepts ONLY one state and one code. url.Values.Get
// silently picks the first value, and extra provider/attacker parameters must
// never influence a credential-bearing callback.
func cloudOAuthCallbackCode(query url.Values, expectedState string) (string, bool) {
	states, hasState := query["state"]
	codes, hasCode := query["code"]
	if !hasState || !hasCode || len(query) != 2 || len(states) != 1 || len(codes) != 1 ||
		expectedState == "" || states[0] != expectedState {
		return "", false
	}
	code := codes[0]
	if code == "" || len(code) > 2048 || strings.TrimSpace(code) != code ||
		strings.ContainsAny(code, "\r\n") {
		return "", false
	}
	return code, true
}

// academyExchangeYandexOAuthCode is the sole token exchange path. The injected
// client/endpoint allow transport-level tests without live provider credentials.
// Never follow a redirect carrying an authorization code or client secret.
func academyExchangeYandexOAuthCode(ctx context.Context, client *http.Client, endpoint, id, secret, code, verifier string) (string, error) {
	if client == nil || endpoint == "" || id == "" || secret == "" || code == "" || verifier == "" {
		return "", academy.ErrUnavailable
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", academy.ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(id, secret)
	protected := *client
	protected.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := protected.Do(req)
	if err != nil {
		return "", academy.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", academy.ErrDenied
	}
	// Read the complete bounded response: a valid JSON prefix followed by
	// junk or oversized provider output must not be accepted as credentials.
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return "", academy.ErrUnavailable
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if json.Unmarshal(raw, &token) != nil || token.AccessToken == "" ||
		len(token.AccessToken) > 8192 || strings.TrimSpace(token.AccessToken) != token.AccessToken ||
		strings.ContainsAny(token.AccessToken, "\r\n") ||
		(token.TokenType != "" && !strings.EqualFold(token.TokenType, "bearer")) {
		return "", academy.ErrUnavailable
	}
	return token.AccessToken, nil
}

func registerAcademyYandexOAuth(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	const cookieName = "apgic_academy_yandex_oauth"
	mux.HandleFunc("GET /v1/academy/author/cloud/yandex/connect", func(w http.ResponseWriter, r *http.Request) {
		returnCourse, returnVersion, validReturn := cloudOAuthEditorReturnIDs(r.URL.RawQuery)
		if !validReturn {
			academyHTTPError(w, r, academy.ErrInvalid)
			return
		}
		actor, ok := academyIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		id, _, redirect, err := academyYandexOAuthConfig()
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		key, err := cloudStateKey()
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		verifier, err := cloudRandom(32)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		nonce, err := cloudRandom(24)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		value, err := cloudSeal(cloudOAuthState{Actor: actor, Verifier: verifier, Nonce: nonce, Expires: time.Now().Add(10 * time.Minute).Unix(), ReturnCourse: returnCourse, ReturnVersion: returnVersion}, key)
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/v1/academy/author/cloud/yandex", MaxAge: 600, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		challenge := sha256.Sum256([]byte(verifier))
		q := url.Values{"response_type": {"code"}, "client_id": {id}, "redirect_uri": {redirect}, "state": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "https://oauth.yandex.ru/authorize?"+q.Encode(), http.StatusFound)
	})
	mux.HandleFunc("GET /v1/academy/author/cloud/yandex/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(cookieName)
		http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/v1/academy/author/cloud/yandex", Value: "", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		if err != nil {
			academyHTTPError(w, r, academy.ErrDenied)
			return
		}
		key, err := cloudStateKey()
		if err != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		state, err := cloudOpen(cookie.Value, key)
		code, validCode := cloudOAuthCallbackCodeFromRaw(r.URL.RawQuery, state.Nonce)
		if err != nil || !validCode {
			academyHTTPError(w, r, academy.ErrDenied)
			return
		}
		actor, authenticated := academyIdentity(w, r, sessions, sessionErr)
		if !authenticated {
			return
		}
		if !cloudOAuthActorMatches(state, actor) {
			academyHTTPError(w, r, academy.ErrDenied)
			return
		}
		writer, ok := store.(academyCloudCredentialWriter)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		id, secret, _, err := academyYandexOAuthConfig()
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		// OAuth codes and client secrets must not traverse an environment
		// proxy or a DNS-rebound private endpoint.
		client := academy.NewYandexSecureClient(12 * time.Second)
		accessToken, err := academyExchangeYandexOAuthCode(r.Context(), client, "https://oauth.yandex.ru/token", id, secret, code, state.Verifier)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		if writer.AcademyStoreCloudCredential(state.Actor, "YANDEX_DISK", accessToken) != nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		http.Redirect(w, r, cloudOAuthCallbackDestination(state), http.StatusSeeOther)
	})
}
