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
	"net"
	"sync"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const accountDevicePairTTL = 5 * time.Minute

// AccountDevicePairingStore is the single durable account transfer boundary.
// There is no identity merge, no shared device cookie and no role-specific path.
type accountDevicePairingStore interface {
	CreateAccountDevicePair(id, codeDigest, proofDigest string, expires time.Time) error
	ApproveAccountDevicePair(codeDigest, identity string) (bool, error)
	ClaimAccountDevicePair(id, proofDigest string) (string, bool, error)
	RegisterAccountPairedSession(id, actor, digest string, expires time.Time) error
	AccountPairedSessionActive(actor, digest string) (bool, error)
	ListAccountPairedSessions(actor string) ([]identity.PairedDevice, error)
	RevokeAccountPairedSession(actor, id string) (bool, error)
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
	// Next.js proxies /v1 through localhost and intentionally strips Host.
	// Match the browser's Origin against the explicit public first-party
	// allowlist, not the internal upstream Host or X-Forwarded-* headers.
	origin := r.Header.Get("Origin")
	return (origin == "https://apgic.ru" || origin == "https://www.apgic.ru") &&
		r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

// Anonymous starts have a bounded per-source budget. The public request
// reaches Go only through the local Next proxy; it forwards Nginx's
// sanitized X-Real-IP header. Avoid unbounded in-memory IP maps.
type accountPairStartWindow struct {until time.Time; count int}
type accountPairStartLimiter struct {
 mu sync.Mutex
 windows map[string]accountPairStartWindow
}
func (l *accountPairStartLimiter) allow(r *http.Request) bool {
 host,_,err:=net.SplitHostPort(r.RemoteAddr)
 if err!=nil {host=r.RemoteAddr}
 if parsed:=net.ParseIP(host); parsed!=nil && parsed.IsLoopback(){
  if raw:=r.Header.Get("X-Real-IP");net.ParseIP(raw)!=nil {host=raw}
 }
 id:=accountPairDigest(host)
 now:=time.Now()
 l.mu.Lock();defer l.mu.Unlock()
 if l.windows==nil {l.windows=make(map[string]accountPairStartWindow)}
 if len(l.windows)>=4096 {
  for key,item:=range l.windows {if now.After(item.until) {delete(l.windows,key)}}
 }
 item:=l.windows[id]
 if !now.Before(item.until) {item=accountPairStartWindow{until:now.Add(time.Minute)}}
 if item.count>=6 || len(l.windows)>=4096 && item.count==0 {return false}
 item.count++
 l.windows[id]=item
 return true
}

func registerAccountDevicePairing(mux *http.ServeMux, store accountDevicePairingStore, sessions *clientSessionManager, sessionErr error) {
	limiter:=&accountPairStartLimiter{}
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
		if !limiter.allow(r){fail(w,r,http.StatusTooManyRequests,"ACCOUNT_PAIRING_RATE_LIMIT","Слишком много запросов подключения. Повторите позже.");return}
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
		cookie, err := sessions.issuePaired(actor)
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось выпустить сессию.")
			return
		}
		deviceID, err := persistentid.New()
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось зарегистрировать устройство.")
			return
		}
		if err = store.RegisterAccountPairedSession(deviceID, actor, accountPairDigest(cookie.Value), cookie.Expires); err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Не удалось зарегистрировать устройство.")
			return
		}
		http.SetCookie(w, cookie)
		writeJSON(w, 200, map[string]any{"connected": true})
	})

	mux.HandleFunc("GET /v1/account/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if store == nil || sessions == nil || sessionErr != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Список устройств недоступен.")
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionErr, "")
		if !ok {
			return
		}
		devices, err := store.ListAccountPairedSessions(actor)
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Список устройств недоступен.")
			return
		}
		writeJSON(w, 200, map[string]any{"devices": devices})
	})
	mux.HandleFunc("POST /v1/account/devices/{deviceID}/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) {
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionErr, "")
		if !ok {
			return
		}
		deviceID := r.PathValue("deviceID")
		if !accountPairIDPattern.MatchString(deviceID) {
			fail(w, r, 400, "ACCOUNT_PAIRING_INVALID", "Неверный идентификатор устройства.")
			return
		}
		revoked, err := store.RevokeAccountPairedSession(actor, deviceID)
		if err != nil {
			fail(w, r, 503, "ACCOUNT_PAIRING_UNAVAILABLE", "Отзыв устройства недоступен.")
			return
		}
		if !revoked {
			fail(w, r, 404, "ACCOUNT_PAIRING_NOT_FOUND", "Устройство не найдено.")
			return
		}
		writeJSON(w, 200, map[string]any{"revoked": true})
	})
}
