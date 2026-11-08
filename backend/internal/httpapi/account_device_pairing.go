package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const accountDevicePairTTL = 5 * time.Minute

// AccountDevicePairingStore is the single durable account transfer boundary.
// There is no identity merge, no shared device cookie and no role-specific path.
type accountDevicePairingStore interface {
	CreateAccountDevicePair(id, codeDigest, proofDigest string, expires time.Time) error
	ApproveAccountDevicePair(codeDigest, identity string) (bool, error)
	ClaimAccountDevicePair(id, proofDigest string) (string, bool, error)
}

var accountPairCodePattern = regexp.MustCompile(`^[A-F0-9]{16}$`)
var accountPairIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func accountPairDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func accountPairBody(r *http.Request, target any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

func accountPairOrigin(r *http.Request) bool {
	// These credential-issuing endpoints require an actual same-origin browser
	// POST. No OAuth callback, cross-site form, or Origin-less mutation accepted.
	return r.Header.Get("Origin") == "https://"+r.Host &&
		r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

func registerAccountDevicePairing(mux *http.ServeMux, store accountDevicePairingStore, sessions *clientSessionManager, sessionErr error) {
	fail := func(w http.ResponseWriter, r *http.Request, status int, code, message string) {
		writeDemandError(w, r, status, code, message, false, nil)
	}
	available := func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Cache-Control", "no-store")
		if store == nil || sessions == nil || sessionErr != nil {
			fail(w, r, http.StatusServiceUnavailable, "ACCOUNT_PAIRING_UNAVAILABLE", "Вход с другого устройства пока недоступен.")
			return false
		}
		if !accountPairOrigin(r) {
			fail(w, r, http.StatusForbidden, "ACCOUNT_PAIRING_ORIGIN", "Операцию можно выполнить только с сайта APGIC.")
			return false
		}
		return true
	}
	targetIsAnonymous := func(w http.ResponseWriter, r *http.Request) bool {
		// A target that already has an identity must not silently lose its
		// own organizations, courses, payments or patient data.
		_, err := r.Cookie(clientSessionCookieName)
		if err == nil || !errors.Is(err, http.ErrNoCookie) {
			fail(w, r, http.StatusConflict, "ACCOUNT_PAIRING_EXISTING_SESSION", "На устройстве уже есть сессия. Переключение аккаунта без сохранения доступа к старому запрещено.")
			return false
		}
		return true
	}

	mux.HandleFunc("POST /v1/account/devices/pair/start", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) || !targetIsAnonymous(w, r) {
			return
		}
		if r.ContentLength > 2 { // no browser-controlled identity or redirect body
			fail(w, r, http.StatusBadRequest, "ACCOUNT_PAIRING_INVALID", "Некорректный запрос.")
			return
		}
		id, err := persistentid.New()
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось начать подключение.")
			return
		}
		codeBytes := make([]byte, 8)
		proofBytes := make([]byte, 32)
		if _, err = rand.Read(codeBytes); err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось начать подключение.")
			return
		}
		if _, err = rand.Read(proofBytes); err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось начать подключение.")
			return
		}
		code := strings.ToUpper(hex.EncodeToString(codeBytes))
		proof := base64.RawURLEncoding.EncodeToString(proofBytes)
		expires := time.Now().UTC().Add(accountDevicePairTTL)
		if err = store.CreateAccountDevicePair(id, accountPairDigest(code), accountPairDigest(proof), expires); err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось начать подключение.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"pair_id": id, "code": code, "proof": proof, "expires_at": expires.Format(time.RFC3339)})
	})
	mux.HandleFunc("POST /v1/account/devices/pair/approve", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) {
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionErr, "")
		if !ok {
			return
		}
		var request struct {
			Code string `json:"code"`
		}
		if !accountPairBody(r, &request) || !accountPairCodePattern.MatchString(request.Code) {
			fail(w, r, 400, "ACCOUNT_PAIRING_INVALID", "Введите код с другого устройства.")
			return
		}
		approved, err := store.ApproveAccountDevicePair(accountPairDigest(request.Code), actor)
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Подтверждение временно недоступно.")
			return
		}
		if !approved {
			fail(w, r, 409, "ACCOUNT_PAIRING_NOT_FOUND", "Код не найден, истёк или уже использован.")
			return
		}
		writeJSON(w, 200, map[string]any{"approved": true})
	})
	mux.HandleFunc("POST /v1/account/devices/pair/claim", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) || !targetIsAnonymous(w, r) {
			return
		}
		var request struct {
			PairID string `json:"pair_id"`
			Proof  string `json:"proof"`
		}
		if !accountPairBody(r, &request) || !accountPairIDPattern.MatchString(request.PairID) ||
			len(request.Proof) != 43 {
			fail(w, r, 400, "ACCOUNT_PAIRING_INVALID", "Недействительный запрос подключения.")
			return
		}
		// Proof is never disclosed to the approving device, logs or URL.
		actor, ok, err := store.ClaimAccountDevicePair(request.PairID, accountPairDigest(request.Proof))
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Подключение временно недоступно.")
			return
		}
		if !ok {
			fail(w, r, 409, "ACCOUNT_PAIRING_PENDING", "Подтвердите подключение на первом устройстве или создайте новый код.")
			return
		}
		// A deletion between authorization and claim cannot resurrect an identity.
		if sessions.deletions != nil {
			deleted, err := sessions.deletions.ClientIdentityDeleted(r.Context(), actor)
			if err != nil || deleted {
				fail(w, r, 403, "ACCOUNT_PAIRING_DENIED", "Доступ к аккаунту недоступен.")
				return
			}
		}
		cookie, err := sessions.issue(actor)
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось выпустить сессию.")
			return
		}
		http.SetCookie(w, cookie)
		writeJSON(w, 200, map[string]any{"connected": true})
	})
}
